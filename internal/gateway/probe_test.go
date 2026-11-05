package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRealHTTPProbeAcceptsAuthenticatedEmptyCatalog(t *testing.T) {
	caps := &mcp.ServerCapabilities{}
	caps.AddExtension(noxmcp.DiscoveryExtension, noxmcp.DiscoveryCapability())
	server := mcp.NewServer(&mcp.Implementation{Name: "test-gateway", Version: "test"}, &mcp.ServerOptions{Capabilities: caps, SupportedProtocolVersions: []string{noxmcp.DiscoveryProtocol}})
	if err := noxmcp.InstallDiscovery(server, noxmcp.DiscoveryOptions{ServerID: "test-gateway", ResolveView: func(context.Context, *mcp.RequestExtra, []*mcp.Tool) (string, []string, []string, noxmcp.JSON, error) {
		return "authorized-empty", nil, nil, nil, nil
	}}); err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	key := "noxsync_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			http.Error(w, "denied", 401)
			return
		}
		if r.Method != "POST" {
			t.Error("unexpected GET/subscription or retry", r.Method)
		}
		handler.ServeHTTP(w, r)
	}))
	defer peer.Close()
	report, err := Probe(context.Background(), peer.URL, key, "test")
	if err != nil || report.ServerName != "test-gateway" || report.ToolCount != 0 || report.Protocol != noxmcp.DiscoveryProtocol {
		t.Fatal(report, err)
	}
}

func TestHTTPProbeDoesNotFollowRedirectsOrConfuseOutagesWithBadKeys(t *testing.T) {
	for _, status := range []int{401, 403, 503} {
		peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "safe peer failure", status) }))
		_, err := Probe(context.Background(), peer.URL, "test-key", "test")
		peer.Close()
		if err == nil || errors.Is(err, ErrRejected) != (status == 401) {
			t.Fatal("wrong authentication classification", status, err)
		}
	}
	var followed atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL+"/mcp", 302) }))
	defer source.Close()
	if _, err := Probe(context.Background(), source.URL, "test-key", "test"); !errors.Is(err, ErrRedirect) || followed.Load() != 0 {
		t.Fatal("redirect forwarded a credential", err)
	}
	tlsPeer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted TLS reached handler") }))
	defer tlsPeer.Close()
	if _, err := Probe(context.Background(), tlsPeer.URL, "test-key", "test"); err == nil || errors.Is(err, ErrRejected) {
		t.Fatal("invalid TLS accepted or treated as rejected key", err)
	}
}

func TestRealHTTPDiscoveryKeepsCanonicalMenuDataWithoutCallingTools(t *testing.T) {
	caps := &mcp.ServerCapabilities{}
	caps.AddExtension(noxmcp.DiscoveryExtension, noxmcp.DiscoveryCapability())
	server := mcp.NewServer(&mcp.Implementation{Name: "test-gateway", Version: "test"}, &mcp.ServerOptions{Capabilities: caps, SupportedProtocolVersions: []string{noxmcp.DiscoveryProtocol}})
	var calls atomic.Int32
	server.AddTool(&mcp.Tool{Name: "backend-list", InputSchema: map[string]any{"type": "object"}, Meta: map[string]any{"cli": "vault list", "nox/gateway": map[string]any{"version": "1.0", "productId": "nox-backend"}}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return &mcp.CallToolResult{}, nil
	})
	if err := noxmcp.InstallDiscovery(server, noxmcp.DiscoveryOptions{ServerID: "test-gateway", Descriptors: map[string]noxmcp.JSON{"backend-list": {"version": "1.0", "navigation": map[string]any{"group": "Vaults", "label": "List"}}}, ResolveView: func(context.Context, *mcp.RequestExtra, []*mcp.Tool) (string, []string, []string, noxmcp.JSON, error) {
		return "authorized", nil, nil, noxmcp.JSON{"products": []any{map[string]any{"productId": "nox-backend", "displayName": "NoX Backend"}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	peer := httptest.NewServer(handler)
	defer peer.Close()
	catalog, err := Discover(context.Background(), peer.URL, "test-key", "test")
	if err != nil || catalog.Report.ToolCount != 1 {
		t.Fatal(catalog.Report, err)
	}
	definition := catalog.Definitions["backend-list"]
	if definition["tool"].(map[string]any)["_meta"].(map[string]any)["cli"] != "vault list" || definition["cli"] == nil || len(catalog.Presentation) == 0 {
		t.Fatal("human menu data lost on actual MCP exchange")
	}
	if calls.Load() != 0 {
		t.Fatal("discovery executed a business tool")
	}
}
