package gateway

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Report struct {
	ServerName    string
	ServerVersion string
	Protocol      string
	ToolCount     int
}

// Probe performs a real authenticated discovery exchange, including empty catalogs.
// It returns only public status. The session, credential and cache are not retained.
func Probe(ctx context.Context, raw, key, version string) (Report, error) {
	catalog, err := Discover(ctx, raw, key, version)
	return catalog.Report, err
}

// Discover authenticates and reads a complete validated snapshot. It executes no tools.
func Discover(ctx context.Context, raw, key, version string) (Catalog, error) {
	origin, err := Origin(raw)
	if err != nil {
		return Catalog{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, err := connect(ctx, origin, key, version, nil)
	if err != nil {
		return Catalog{}, err
	}
	defer conn.Close()
	session, info := conn.session, conn.session.InitializeResult()
	catalog, err := readCatalog(ctx, noxmcp.DiscoverySessionRequest(session))
	if err != nil {
		if err == ErrProtocol {
			return Catalog{}, ErrProtocol
		}
		return Catalog{}, conn.failure(ctx)
	}
	catalog.Partition = CredentialPartition(origin, key)
	catalog.Report = Report{ServerName: strings.ReplaceAll(info.ServerInfo.Name, key, "[redacted]"), ServerVersion: strings.ReplaceAll(info.ServerInfo.Version, key, "[redacted]"), Protocol: info.ProtocolVersion, ToolCount: len(catalog.Definitions)}
	return catalog, nil
}

type connection struct {
	session   *mcp.ClientSession
	transport *privateTransport
}

func (c *connection) Close() { _ = c.session.Close(); c.transport.base.CloseIdleConnections() }
func (c *connection) failure(ctx context.Context) error {
	if c.transport.rejected.Load() {
		return ErrRejected
	}
	if c.transport.redirected.Load() {
		return ErrRedirect
	}
	if c.transport.privateFailure.Load() {
		return ErrPrivate
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c.transport.networkFailure.Load() {
		return ErrUnavailable
	}
	return ErrRequest
}
func connect(ctx context.Context, origin, key, version string, progress func(*mcp.ProgressNotificationParams)) (*connection, error) {
	transport := newTransport(origin, key, net.DefaultResolver, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)
	transport.base.ResponseHeaderTimeout = 2 * time.Minute
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { transport.redirected.Store(true); return ErrRedirect }}
	client, err := newClient(version, progress)
	if err != nil {
		transport.base.CloseIdleConnections()
		return nil, ErrProtocol
	}
	failed := func() (*connection, error) {
		if transport.rejected.Load() {
			return nil, ErrRejected
		}
		if transport.redirected.Load() {
			return nil, ErrRedirect
		}
		if transport.privateFailure.Load() {
			return nil, ErrPrivate
		}
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}
		if ctx.Err() != nil {
			return nil, ErrUnavailable
		}
		if transport.networkFailure.Load() {
			return nil, ErrUnavailable
		}
		return nil, ErrRequest
	}
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: origin + "/mcp", HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
	if err != nil {
		transport.base.CloseIdleConnections()
		return failed()
	}

	valid := false
	defer func() {
		if !valid {
			_ = session.Close()
			transport.base.CloseIdleConnections()
		}
	}()
	info := session.InitializeResult()
	if info == nil || info.ServerInfo == nil || info.Capabilities == nil || info.ProtocolVersion != noxmcp.DiscoveryProtocol {
		transport.base.CloseIdleConnections()
		return nil, ErrProtocol
	}
	capability, ok := info.Capabilities.Extensions[noxmcp.DiscoveryExtension].(map[string]any)
	if !ok {
		transport.base.CloseIdleConnections()
		return nil, ErrProtocol
	}
	supported := false
	if versions, ok := capability["versions"].([]any); ok {
		for _, version := range versions {
			if version == noxmcp.DiscoveryVersion {
				supported = true
			}
		}
	}
	if !supported {
		transport.base.CloseIdleConnections()
		return nil, ErrProtocol
	}

	valid = true
	return &connection{session: session, transport: transport}, nil
}
