// Package gateway contains the outgoing MCP client boundary.
package gateway

import (
	"context"
	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewClient prepares the official client for negotiated NoX discovery. It does
// not connect, accept a credential or enable any incoming MCP server/listener.
// Discover supplies the authenticated private HTTP transport.
func NewClient(version string) (*mcp.Client, error) { return newClient(version, nil) }

func newClient(version string, progress func(*mcp.ProgressNotificationParams)) (*mcp.Client, error) {
	capabilities := &mcp.ClientCapabilities{}
	capabilities.AddExtension(noxmcp.DiscoveryExtension, noxmcp.JSON{"version": noxmcp.DiscoveryVersion})
	client := mcp.NewClient(&mcp.Implementation{Name: "nox-cli", Version: version}, &mcp.ClientOptions{Capabilities: capabilities, MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true}, ProgressNotificationHandler: func(_ context.Context, r *mcp.ProgressNotificationClientRequest) {
		if progress != nil {
			progress(r.Params)
		}
	}})
	if err := noxmcp.RegisterDiscoveryClient(client); err != nil {
		return nil, err
	}
	return client, nil
}
