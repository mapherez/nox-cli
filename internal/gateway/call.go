package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	ErrStale   = errors.New("The definition or access changed. Refresh the catalog and review the action again. Nothing will be replayed.")
	ErrAccess  = errors.New("This tool is not available to your current account.")
	ErrUnknown = errors.New("Execution outcome is unknown. Inspect the app before trying again; the operation was not retried.")
)

type Call struct {
	Catalog           Catalog
	Name, Fingerprint string
	Arguments         map[string]any
	ReadOnly          bool
	Revalidate        bool
	Expected          map[string]string
	Progress          func(string)
}

// Execute performs exactly one guarded business call. Human time in a form does
// not consume its two-minute network budget. No retry or multi-round-trip replay.
func Execute(parent context.Context, raw, key, version string, call Call) (*mcp.CallToolResult, error) {
	origin, err := Origin(raw)
	if err != nil {
		return nil, err
	}
	if call.Catalog.Partition == "" || call.Catalog.Partition != CredentialPartition(origin, key) {
		return nil, ErrStale
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	conn, err := connect(ctx, origin, key, version, func(p *mcp.ProgressNotificationParams) {
		if call.Progress != nil {
			call.Progress(strings.ReplaceAll(p.Message, key, "[redacted]"))
		}
	})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	definition := call.Catalog.Definitions[call.Name]
	if definition == nil || definition["fingerprint"] != call.Fingerprint || call.Fingerprint == "" {
		return nil, ErrStale
	}
	// A fresh guarded call is sufficient within TTL. Expired snapshots are verified
	// again before dispatch; a changed authorized view requires another review.
	if call.Revalidate || call.Catalog.TTL <= 0 || time.Since(call.Catalog.LoadedAt) >= call.Catalog.TTL {
		fresh, e := readCatalog(ctx, noxmcp.DiscoverySessionRequest(conn.session))
		if e != nil {
			return nil, conn.failure(ctx)
		}
		current := fresh.Definitions[call.Name]
		if fresh.ServerID != call.Catalog.ServerID || fresh.ViewID != call.Catalog.ViewID || current == nil || current["fingerprint"] != call.Fingerprint {
			return nil, ErrStale
		}
		if current["availability"] == "unavailable" {
			return nil, ErrAccess
		}
		for name, fp := range call.Expected {
			d := fresh.Definitions[name]
			if d == nil || d["fingerprint"] != fp {
				return nil, ErrStale
			}
			if d["availability"] == "unavailable" {
				return nil, ErrAccess
			}
		}

	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	conn.transport.record.Store(true)
	params := &mcp.CallToolParams{Name: call.Name, Arguments: call.Arguments, Meta: mcp.Meta(noxmcp.DiscoveryMeta())}
	params.Meta[noxmcp.DiscoveryExtension] = map[string]any{"expectedFingerprint": call.Fingerprint}
	params.SetProgressToken("nox-cli-call")
	result, err := conn.session.CallTool(ctx, params)
	if err != nil {
		if conn.transport.rejected.Load() {
			return nil, ErrRejected
		}
		var rpc *jsonrpc.Error
		if errors.As(err, &rpc) {
			if strings.Contains(rpc.Message, "STALE_DEFINITION") || strings.Contains(rpc.Message, "STALE_CATALOG") {
				return nil, ErrStale
			}
			if rpc.Code == jsonrpc.CodeInvalidParams || rpc.Code == jsonrpc.CodeMethodNotFound {
				return nil, ErrRequest
			}
		}
		if !call.ReadOnly {
			return nil, ErrUnknown
		}
		return nil, conn.failure(ctx)
	}
	if result == nil {
		if !call.ReadOnly {
			return nil, ErrUnknown
		}
		return nil, ErrProtocol
	}
	// Restore structured JSON numbers from the actual wire, before the SDK's any
	// decoding can round integers larger than 2^53. Never parse ordinary text blocks.
	if raw := conn.transport.recorded(); len(raw) > 0 {
		var envelope struct {
			Result struct {
				Structured json.RawMessage `json:"structuredContent"`
			} `json:"result"`
		}
		if json.Unmarshal(raw, &envelope) == nil && len(envelope.Result.Structured) > 0 {
			dec := json.NewDecoder(bytes.NewReader(envelope.Result.Structured))
			dec.UseNumber()
			if dec.Decode(&result.StructuredContent) != nil {
				return nil, ErrProtocol
			}
		}
	}
	// The personal key is never returned to the UI, even if a peer echoes it.
	b, e := json.Marshal(result)
	if e != nil {
		return nil, ErrProtocol
	}
	b = bytes.ReplaceAll(b, []byte(key), []byte("[redacted]"))
	var safe mcp.CallToolResult
	if json.Unmarshal(b, &safe) != nil {
		return nil, ErrProtocol
	}
	if result.StructuredContent != nil {
		var raw struct {
			Structured json.RawMessage `json:"structuredContent"`
		}
		_ = json.Unmarshal(b, &raw)
		dec := json.NewDecoder(bytes.NewReader(raw.Structured))
		dec.UseNumber()
		if dec.Decode(&safe.StructuredContent) != nil {
			return nil, ErrProtocol
		}
	}
	if safe.IsError {
		if body, ok := safe.StructuredContent.(map[string]any); ok {
			switch body["code"] {
			case "STALE_DEFINITION", "STALE_CATALOG", "AUTHORIZATION_INVALIDATED":
				return &safe, ErrStale
			case "FORBIDDEN":
				return &safe, ErrAccess
			case "EXECUTION_OUTCOME_UNCONFIRMED":
				return &safe, ErrUnknown
			}
		}
	}
	return &safe, nil
}
