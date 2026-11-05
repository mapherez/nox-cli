// Package setup coordinates verified settings and the single credential slot.
package setup

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sync"

	"github.com/mapherez/nox-cli/internal/config"
	"github.com/mapherez/nox-cli/internal/credentials"
	"github.com/mapherez/nox-cli/internal/gateway"
)

type ProbeFunc func(context.Context, string, string, string) (gateway.Report, error)

type Manager struct {
	Store    config.Store
	Vault    credentials.Vault
	Probe    ProbeFunc
	Discover func(context.Context, string, string, string) (gateway.Catalog, error)
	Version  string
	Execute  func(context.Context, string, string, string, gateway.Call) (*mcp.CallToolResult, error)
	mu       sync.Mutex
	session  *credentials.Secret
	rejected bool
}

func New(root, version string) *Manager {
	return &Manager{Store: config.Store{Root: root}, Vault: credentials.Native{}, Probe: gateway.Probe, Discover: gateway.Discover, Execute: gateway.Execute, Version: version}
}

func (m *Manager) Settings() (config.Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Store.Load()
}

func (m *Manager) read(settings config.Settings) (credentials.Secret, error) {
	if m.rejected {
		return credentials.Secret{}, credentials.ErrNotFound
	}
	if m.session != nil && m.session.Origin == settings.GatewayOrigin {
		return *m.session, nil
	}
	if settings.GatewayOrigin == "" || settings.SessionOnly || settings.CredentialRef != config.CredentialRef {
		return credentials.Secret{}, credentials.ErrNotFound
	}
	secret, err := m.Vault.Get()
	if err != nil {
		return credentials.Secret{}, err
	}
	// A crash between vault/config writes must never send a key to another origin.
	if secret.Origin != settings.GatewayOrigin {
		return credentials.Secret{}, credentials.ErrNotFound
	}
	return secret, nil
}

func (m *Manager) check(ctx context.Context, secret credentials.Secret) (gateway.Report, error) {
	report, err := m.Probe(ctx, secret.Origin, secret.Key, m.Version)
	if errors.Is(err, gateway.ErrRejected) {
		m.rejected = true
		m.session = nil
	}
	return report, err
}

func (m *Manager) Check(ctx context.Context) (gateway.Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	settings, err := m.Store.Load()
	if err != nil {
		return gateway.Report{}, err
	}
	secret, err := m.read(settings)
	if err != nil {
		return gateway.Report{}, err
	}
	return m.check(ctx, secret)
}

func (m *Manager) Save(ctx context.Context, rawOrigin, key string, sessionOnly bool) (gateway.Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	origin, err := gateway.Origin(rawOrigin)
	if err != nil {
		return gateway.Report{}, err
	}
	previous, err := m.Store.Load()
	if err != nil {
		return gateway.Report{}, err
	}
	usingSaved := key == ""
	if usingSaved {
		if origin != previous.GatewayOrigin {
			return gateway.Report{}, credentials.ErrNotFound
		}
		old, err := m.read(previous)
		if err != nil {
			return gateway.Report{}, err
		}
		key = old.Key
	}
	if err := credentials.ValidateKey(key); err != nil {
		return gateway.Report{}, err
	}
	candidate := credentials.Secret{Origin: origin, Key: key}
	// A failed candidate must not invalidate an already configured different key.
	report, err := m.Probe(ctx, origin, key, m.Version)
	if err != nil {
		if usingSaved && errors.Is(err, gateway.ErrRejected) {
			m.rejected = true
			m.session = nil
		}
		return gateway.Report{}, err
	}
	if ctx.Err() != nil {
		return gateway.Report{}, errors.New("Connection check canceled. Settings were not saved.")
	}
	if err := m.Store.Directory(); err != nil {
		return gateway.Report{}, err
	}
	settings := config.Settings{Version: 1, GatewayOrigin: origin, SessionOnly: sessionOnly}
	if sessionOnly {
		// Explicit session-only mode never falls back to a previously persisted key.
		if err := m.Store.Save(settings); err != nil {
			return gateway.Report{}, err
		}
		m.session = &candidate
		m.rejected = false
		return report, nil
	}
	settings.CredentialRef = config.CredentialRef
	oldSecret, oldErr := m.Vault.Get()
	if oldErr != nil && !errors.Is(oldErr, credentials.ErrNotFound) {
		return gateway.Report{}, oldErr
	}
	if err := m.Vault.Set(candidate); err != nil {
		return gateway.Report{}, err
	}
	restore := func() error {
		if oldErr == nil {
			return m.Vault.Set(oldSecret)
		}
		return m.Vault.Delete()
	}
	if ctx.Err() != nil {
		if restore() != nil {
			return gateway.Report{}, errors.New("Saving was interrupted. Re-enter the API key before connecting.")
		}
		return gateway.Report{}, errors.New("Connection check canceled. Settings were not saved.")
	}
	if err := m.Store.Save(settings); err != nil {
		if restore() != nil {
			return gateway.Report{}, errors.New("Unable to finish saving. Re-enter the API key before connecting.")
		}
		return gateway.Report{}, err
	}
	m.session = nil
	m.rejected = false
	return report, nil
}

func (m *Manager) Remove() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	settings, err := m.Store.Load()
	if err != nil {
		return err
	}
	// Clear in-memory admission even when the native vault cannot be reached.
	m.session = nil
	m.rejected = true
	if err := m.Vault.Delete(); err != nil {
		return err
	}
	if settings.GatewayOrigin != "" {
		settings.CredentialRef = ""
		settings.SessionOnly = false
		if err := m.Store.Save(settings); err != nil {
			return err
		}
	}
	return nil
}

// Browse uses exactly the configured credential and the existing rejection rules.
func (m *Manager) Browse(ctx context.Context) (gateway.Catalog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	settings, err := m.Store.Load()
	if err != nil {
		return gateway.Catalog{}, err
	}
	secret, err := m.read(settings)
	if err != nil {
		return gateway.Catalog{}, err
	}
	if m.Discover == nil {
		return gateway.Catalog{}, gateway.ErrProtocol
	}
	catalog, err := m.Discover(ctx, secret.Origin, secret.Key, m.Version)
	if errors.Is(err, gateway.ErrRejected) {
		m.rejected = true
		m.session = nil
	}
	if err == nil {
		catalog.Partition = gateway.CredentialPartition(secret.Origin, secret.Key)
	}
	return catalog, err
}

func (m *Manager) Call(ctx context.Context, call gateway.Call) (*mcp.CallToolResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	settings, err := m.Store.Load()
	if err != nil {
		return nil, err
	}
	secret, err := m.read(settings)
	if err != nil {
		return nil, err
	}
	if call.Catalog.Partition == "" || call.Catalog.Partition != gateway.CredentialPartition(secret.Origin, secret.Key) {
		return nil, gateway.ErrStale
	}
	if m.Execute == nil {
		return nil, gateway.ErrProtocol
	}
	result, err := m.Execute(ctx, secret.Origin, secret.Key, m.Version, call)
	if errors.Is(err, gateway.ErrRejected) {
		m.rejected = true
		m.session = nil
	}
	return result, err
}
