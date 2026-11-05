package gateway

import (
	"context"
	"testing"
	"time"

	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOutgoingClientNegotiatesPublishedDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	caps := &mcp.ServerCapabilities{}
	caps.AddExtension(noxmcp.DiscoveryExtension, noxmcp.DiscoveryCapability())
	server := mcp.NewServer(&mcp.Implementation{Name: "test-peer", Version: "test"}, &mcp.ServerOptions{Capabilities: caps, SupportedProtocolVersions: []string{noxmcp.DiscoveryProtocol}})
	if err := noxmcp.InstallDiscovery(server, noxmcp.DiscoveryOptions{
		ServerID: "test-peer",
		ResolveView: func(context.Context, *mcp.RequestExtra, []*mcp.Tool) (string, []string, []string, noxmcp.JSON, error) {
			return "test-view", nil, nil, nil, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("test-build")
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	identity := ss.InitializeParams().ClientInfo
	if identity.Name != "nox-cli" || identity.Version != "test-build" || session.InitializeResult().ProtocolVersion != noxmcp.DiscoveryProtocol {
		t.Fatalf("unexpected client identity/protocol: %+v", identity)
	}
	cache, err := noxmcp.NewDiscoveryCache("in-memory-test", "test-credential-partition")
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Sync(ctx, noxmcp.DiscoverySessionRequest(session)); err != nil {
		t.Fatal("discovery negotiation/request failed", err)
	}
	if len(cache.Definitions()) != 0 {
		t.Fatal("empty peer unexpectedly published tools")
	}
}
