package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func executionPeer(t *testing.T, handler mcp.ToolHandler) (*mcp.Server, string, *atomic.Int32) {
	t.Helper()
	caps := &mcp.ServerCapabilities{}
	caps.AddExtension(noxmcp.DiscoveryExtension, noxmcp.DiscoveryCapability())
	server := mcp.NewServer(&mcp.Implementation{Name: "test-peer", Version: "test"}, &mcp.ServerOptions{Capabilities: caps, SupportedProtocolVersions: []string{noxmcp.DiscoveryProtocol}})
	var calls atomic.Int32
	server.AddTool(&mcp.Tool{Name: "action", InputSchema: map[string]any{"type": "object"}, Meta: map[string]any{"cli": "action", "nox/gateway": map[string]any{"version": "1.0", "productId": "contract-test", "serverId": "source"}}}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return handler(ctx, r)
	})
	if e := noxmcp.InstallDiscovery(server, noxmcp.DiscoveryOptions{ServerID: "test-peer", ResolveView: func(context.Context, *mcp.RequestExtra, []*mcp.Tool) (string, []string, []string, noxmcp.JSON, error) {
		return "test-account", nil, nil, nil, nil
	}}); e != nil {
		t.Fatal(e)
	}
	httpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	peer := httptest.NewServer(httpHandler)
	t.Cleanup(peer.Close)
	return server, peer.URL, &calls
}
func discoveredCall(t *testing.T, url string) Call {
	t.Helper()
	c, e := Discover(context.Background(), url, "test-key", "test")
	if e != nil {
		t.Fatal(e)
	}
	return Call{Catalog: c, Name: "action", Fingerprint: c.Definitions["action"]["fingerprint"].(string), Arguments: map[string]any{}, Revalidate: true}
}

func TestRealGuardedCallPreservesExactJSONAndRedactsCredential(t *testing.T) {
	_, url, calls := executionPeer(t, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if !strings.Contains(string(r.Params.Arguments), "9007199254740993") {
			t.Error("integer argument rounded", string(r.Params.Arguments))
		}
		guard, _ := r.Params.Meta[noxmcp.DiscoveryExtension].(map[string]any)
		if guard["expectedFingerprint"] == nil {
			t.Error("missing fingerprint guard")
		}
		return &mcp.CallToolResult{StructuredContent: map[string]any{"id": json.Number("9007199254740993"), "echo": "test-key"}, Content: []mcp.Content{&mcp.TextContent{Text: "test-key"}}}, nil
	})
	call := discoveredCall(t, url)
	call.Arguments = map[string]any{"id": json.Number("9007199254740993")}
	result, e := Execute(context.Background(), url, "test-key", "test", call)
	if e != nil {
		t.Fatal(e)
	}
	value := result.StructuredContent.(map[string]any)
	if value["id"] != json.Number("9007199254740993") || value["echo"] != "[redacted]" || calls.Load() != 1 {
		t.Fatal("result precision, secret redaction or single dispatch failed", value, calls.Load())
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "test-key") {
		t.Fatal("credential returned to UI")
	}
}
func TestChangedOrMissingAccessNeverDispatchesOrReplays(t *testing.T) {
	server, url, calls := executionPeer(t, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	call := discoveredCall(t, url)
	server.AddTool(&mcp.Tool{Name: "action", Description: "changed", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return &mcp.CallToolResult{}, nil
	})
	if _, e := Execute(context.Background(), url, "test-key", "test", call); !errors.Is(e, ErrStale) || calls.Load() != 0 {
		t.Fatal("stale review dispatched", e, calls.Load())
	}
}
func TestDomainFailureAndUnknownOutcomeAreDistinctFromKeyRejection(t *testing.T) {
	for _, code := range []string{"FORBIDDEN", "EXECUTION_OUTCOME_UNCONFIRMED", "STALE_DEFINITION", "CONFLICT"} {
		t.Run(code, func(t *testing.T) {
			_, url, calls := executionPeer(t, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{IsError: true, StructuredContent: map[string]any{"code": code}}, nil
			})
			r, e := Execute(context.Background(), url, "test-key", "test", discoveredCall(t, url))
			expected := map[string]error{"FORBIDDEN": ErrAccess, "EXECUTION_OUTCOME_UNCONFIRMED": ErrUnknown, "STALE_DEFINITION": ErrStale}[code]
			if (expected == nil && e != nil) || (expected != nil && !errors.Is(e, expected)) || errors.Is(e, ErrRejected) || r == nil || calls.Load() != 1 {
				t.Fatal("failure misclassified", code, e, calls.Load())
			}
		})
	}
}
func TestCanceledReadAndWriteHaveDifferentOutcomesWithoutReplay(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		t.Run(map[bool]string{true: "read", false: "write"}[readOnly], func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			_, url, calls := executionPeer(t, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				close(started)
				select {
				case <-ctx.Done():
				case <-release:
				}
				return &mcp.CallToolResult{StructuredContent: map[string]any{"late": true}}, nil
			})
			call := discoveredCall(t, url)
			call.ReadOnly = readOnly
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			finished := make(chan error, 1)
			go func() { _, e := Execute(ctx, url, "test-key", "test", call); finished <- e }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("tool never started")
			}
			cancel()
			select {
			case e := <-finished:
				if readOnly && !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
				if !readOnly && !errors.Is(e, ErrUnknown) {
					t.Fatal(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation blocked")
			}
			if calls.Load() != 1 {
				t.Fatal("canceled tool replayed", calls.Load())
			}
		})
	}
}
func TestHTTPKeyRejectionBeforeDispatch(t *testing.T) {
	var revoked atomic.Bool
	server, url, calls := executionPeer(t, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	_ = server
	call := discoveredCall(t, url)
	// A new private ingress simulates the same deployment returning HTTP 401.
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revoked.Store(true)
		_, _ = io.Copy(io.Discard, r.Body)
		http.Error(w, "do not echo credential", 401)
	}))
	defer peer.Close()
	call.Catalog.Partition = CredentialPartition(peer.URL, "test-key")
	if _, e := Execute(context.Background(), peer.URL, "test-key", "test", call); !errors.Is(e, ErrRejected) || calls.Load() != 0 || !revoked.Load() {
		t.Fatal("key rejection not enforced", e)
	}
}
