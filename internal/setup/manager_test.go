package setup

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mapherez/nox-cli/internal/config"
	"github.com/mapherez/nox-cli/internal/credentials"
	"github.com/mapherez/nox-cli/internal/gateway"
)

type vault struct {
	value       credentials.Secret
	writes      int
	unavailable bool
}

func (v *vault) Get() (credentials.Secret, error) {
	if v.unavailable {
		return credentials.Secret{}, credentials.ErrUnavailable
	}
	if v.value.Key == "" {
		return credentials.Secret{}, credentials.ErrNotFound
	}
	return v.value, nil
}
func (v *vault) Set(s credentials.Secret) error {
	if v.unavailable {
		return credentials.ErrUnavailable
	}
	v.value = s
	v.writes++
	return nil
}
func (v *vault) Delete() error {
	if v.unavailable {
		return credentials.ErrUnavailable
	}
	v.value = credentials.Secret{}
	return nil
}
func testKey(value byte) string {
	b := make([]byte, 32)
	b[0] = value
	return "noxsync_" + base64.RawURLEncoding.EncodeToString(b)
}

func TestReplacementIsOneSlotAndOnlyAfterSuccessfulValidation(t *testing.T) {
	ctx := context.Background()
	v := &vault{}
	accepted := testKey(1)
	calls := 0
	m := &Manager{Store: config.Store{Root: filepath.Join(t.TempDir(), "nox-cli")}, Vault: v, Probe: func(_ context.Context, origin, key, version string) (gateway.Report, error) {
		calls++
		if key != accepted {
			return gateway.Report{}, gateway.ErrRejected
		}
		return gateway.Report{ToolCount: 0}, nil
	}}
	if _, err := m.Save(ctx, "http://localhost:5711", accepted, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(ctx, "http://localhost:5711", testKey(2), false); !errors.Is(err, gateway.ErrRejected) || v.writes != 1 || v.value.Key != accepted {
		t.Fatal("failed candidate replaced existing credential", err)
	}
	accepted = testKey(2)
	if _, err := m.Save(ctx, "http://localhost:5711", accepted, false); err != nil || v.writes != 2 || v.value.Key != accepted {
		t.Fatal("key was not replaced in the same slot", err)
	}
	data, _ := os.ReadFile(filepath.Join(m.Store.Root, "config.json"))
	if strings.Contains(string(data), accepted) || strings.Contains(string(data), "noxsync_") {
		t.Fatal("secret persisted outside vault")
	}
	if _, err := m.Save(ctx, "http://localhost:5712", "", false); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatal("saved key could be sent to a different origin", err)
	}
	accepted = testKey(3)
	if _, err := m.Check(ctx); !errors.Is(err, gateway.ErrRejected) {
		t.Fatal("revocation was not reported", err)
	}
	before := calls
	if _, err := m.Check(ctx); !errors.Is(err, credentials.ErrNotFound) || calls != before {
		t.Fatal("rejected key was silently reused", err)
	}
	if _, err := m.Save(ctx, "http://localhost:5711", accepted, false); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(); err != nil || v.value.Key != "" {
		t.Fatal("key not removed", err)
	}
}

func TestUnavailableVaultAndSessionOnlyDoNotWritePlaintext(t *testing.T) {
	key := testKey(1)
	v := &vault{unavailable: true}
	m := &Manager{Store: config.Store{Root: t.TempDir()}, Vault: v, Probe: func(context.Context, string, string, string) (gateway.Report, error) { return gateway.Report{}, nil }}
	if _, err := m.Save(context.Background(), "http://localhost", key, false); !errors.Is(err, credentials.ErrUnavailable) {
		t.Fatal("vault failure was bypassed", err)
	}
	if _, err := m.Save(context.Background(), "http://localhost", key, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.session = nil
	if _, err := m.Check(context.Background()); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatal("session-only key survived session end", err)
	}
	data, _ := os.ReadFile(filepath.Join(m.Store.Root, "config.json"))
	if strings.Contains(string(data), key) {
		t.Fatal("session key stored in file")
	}
}

func TestSettingsFailureRestoresTheSameCredentialSlot(t *testing.T) {
	root := t.TempDir()
	old := credentials.Secret{Origin: "http://localhost", Key: testKey(1)}
	v := &vault{value: old}
	m := &Manager{Store: config.Store{Root: root}, Vault: v, Probe: func(context.Context, string, string, string) (gateway.Report, error) {
		if err := os.Mkdir(filepath.Join(root, "config.json"), 0700); err != nil {
			t.Fatal(err)
		}
		return gateway.Report{}, nil
	}}
	if _, err := m.Save(context.Background(), old.Origin, testKey(2), false); err == nil {
		t.Fatal("settings failure reported success")
	}
	if v.value != old {
		t.Fatal("settings failure did not restore the original vault value")
	}
}

func TestBlankSaveDoesNotRetryARejectedSavedKey(t *testing.T) {
	root := t.TempDir()
	v := &vault{value: credentials.Secret{Origin: "http://localhost", Key: testKey(1)}}
	store := config.Store{Root: root}
	if err := store.Save(config.Settings{Version: 1, GatewayOrigin: v.value.Origin, CredentialRef: config.CredentialRef}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m := &Manager{Store: store, Vault: v, Probe: func(context.Context, string, string, string) (gateway.Report, error) {
		calls++
		return gateway.Report{}, gateway.ErrRejected
	}}
	if _, err := m.Save(context.Background(), v.value.Origin, "", false); !errors.Is(err, gateway.ErrRejected) {
		t.Fatal(err)
	}
	if _, err := m.Save(context.Background(), v.value.Origin, "", false); !errors.Is(err, credentials.ErrNotFound) || calls != 1 {
		t.Fatal("rejected stored key retried without new input", err)
	}
}

func TestCanceledCandidateDoesNotPersist(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := &vault{}
	root := filepath.Join(t.TempDir(), "nox-cli")
	m := &Manager{Store: config.Store{Root: root}, Vault: v, Probe: func(context.Context, string, string, string) (gateway.Report, error) {
		cancel()
		return gateway.Report{}, nil
	}}
	if _, err := m.Save(ctx, "http://localhost", testKey(1), false); err == nil {
		t.Fatal("canceled candidate was saved")
	}
	if v.writes != 0 {
		t.Fatal("canceled connection changed vault")
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled connection created settings")
	}
}

func TestBrowseUsesCurrentCredentialAndStopsAfterRejection(t *testing.T) {
	ctx := context.Background()
	key := testKey(7)
	v := &vault{}
	m := New(filepath.Join(t.TempDir(), "nox-cli"), "test")
	m.Vault = v
	m.Probe = func(context.Context, string, string, string) (gateway.Report, error) { return gateway.Report{}, nil }
	if _, err := m.Save(ctx, "http://localhost:5711", key, false); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m.Discover = func(_ context.Context, origin, actual, version string) (gateway.Catalog, error) {
		calls++
		if origin != "http://localhost:5711" || actual != key {
			t.Fatal("wrong credential or origin")
		}
		return gateway.Catalog{}, gateway.ErrRejected
	}
	if _, err := m.Browse(ctx); !errors.Is(err, gateway.ErrRejected) {
		t.Fatal(err)
	}
	if _, err := m.Browse(ctx); !errors.Is(err, credentials.ErrNotFound) || calls != 1 {
		t.Fatal("rejected key retried", calls, err)
	}
}

func TestOldFormCannotUseReplacementCredentialContext(t *testing.T) {
	m := New(filepath.Join(t.TempDir(), "nox-cli"), "test")
	v := &vault{}
	m.Vault = v
	m.Probe = func(context.Context, string, string, string) (gateway.Report, error) { return gateway.Report{}, nil }
	if _, e := m.Save(context.Background(), "http://localhost:5711", testKey(1), false); e != nil {
		t.Fatal(e)
	}
	old := gateway.Catalog{Partition: gateway.CredentialPartition("http://localhost:5711", testKey(1))}
	if _, e := m.Save(context.Background(), "http://localhost:5711", testKey(2), false); e != nil {
		t.Fatal(e)
	}
	calls := 0
	m.Execute = func(context.Context, string, string, string, gateway.Call) (*mcp.CallToolResult, error) {
		calls++
		return &mcp.CallToolResult{}, nil
	}
	if _, e := m.Call(context.Background(), gateway.Call{Catalog: old}); !errors.Is(e, gateway.ErrStale) || calls != 0 {
		t.Fatal("old review dispatched under replacement key", e, calls)
	}
}
