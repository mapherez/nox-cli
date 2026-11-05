package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	noxmcp "github.com/mapherez/nox-mcp/go"
)

// Catalog is a complete verified read-only snapshot, never an authorization grant.
// It contains no connection, native credential, or service token.
type Catalog struct {
	Partition    string
	Report       Report
	Definitions  map[string]noxmcp.JSON
	Presentation noxmcp.JSON
	ServerID     string
	ViewID       string
	Fingerprint  string
	LoadedAt     time.Time
	TTL          time.Duration
}

// readCatalog uses published schemas and JCS/SHA-256 for a fresh snapshot. CLI
// feature interpretation belongs to each menu action, not the whole snapshot:
// a future required interaction can disable one action without hiding all tools.
func readCatalog(ctx context.Context, request noxmcp.DiscoveryRequest) (Catalog, error) {
	invalid := func() (Catalog, error) { return Catalog{}, ErrProtocol }
	first, err := request(ctx, "nox/tools/catalog", noxmcp.JSON{"_meta": noxmcp.DiscoveryMeta()})
	if err != nil {
		return Catalog{}, err
	}
	if !validFrame("discovery-catalog", first) || first["notModified"] == true {
		return invalid()
	}
	serverID, _ := first["serverId"].(string)
	viewID, _ := first["viewId"].(string)
	hash, _ := first["catalog"].(string)
	entries, ok := first["tools"].([]any)
	if !ok {
		return invalid()
	}
	entries = append([]any{}, entries...)
	page := first
	for page["nextCursor"] != nil {
		cursor, ok := page["nextCursor"].(float64)
		if !ok || int(cursor) != len(entries) || len(entries) >= 100000 {
			return invalid()
		}
		page, err = request(ctx, "nox/tools/catalog", noxmcp.JSON{"catalog": hash, "cursor": cursor, "_meta": noxmcp.DiscoveryMeta()})
		if err != nil {
			return Catalog{}, err
		}
		if !validFrame("discovery-catalog", page) || page["catalog"] != hash || page["serverId"] != serverID || page["viewId"] != viewID {
			return invalid()
		}
		rows, ok := page["tools"].([]any)
		if !ok || len(rows) == 0 {
			return invalid()
		}
		entries = append(entries, rows...)
	}
	if len(entries) > 100000 {
		return invalid()
	}
	identity := noxmcp.JSON{"serverId": serverID, "viewId": viewID, "tools": entries}
	if presentation, ok := first["presentation"]; ok {
		identity["presentation"] = presentation
	}
	actual, err := noxmcp.Fingerprint(identity)
	if err != nil || actual != hash {
		return invalid()
	}
	expected := map[string]string{}
	availability := map[string]any{}
	for _, value := range entries {
		row, ok := value.(map[string]any)
		if !ok {
			return invalid()
		}
		name, _ := row["name"].(string)
		fingerprint, _ := row["fingerprint"].(string)
		if name == "" || expected[name] != "" {
			return invalid()
		}
		expected[name] = fingerprint
		if value, ok := row["availability"]; ok {
			availability[name] = value
		}
	}
	definitions := map[string]noxmcp.JSON{}
	for start := 0; start < len(entries); start += 64 {
		batch := entries[start:min(start+64, len(entries))]
		refs := []any{}
		for _, value := range batch {
			row := value.(map[string]any)
			refs = append(refs, noxmcp.JSON{"name": row["name"], "fingerprint": row["fingerprint"]})
		}
		response, err := request(ctx, "nox/tools/definitions", noxmcp.JSON{"catalog": hash, "tools": refs, "_meta": noxmcp.DiscoveryMeta()})
		if err != nil {
			return Catalog{}, err
		}
		if !validFrame("discovery-definitions", response) || response["catalog"] != hash {
			return invalid()
		}
		received, ok := response["definitions"].([]any)
		if !ok || len(received) != len(batch) {
			return invalid()
		}
		requested := map[string]bool{}
		for _, ref := range refs {
			requested[ref.(map[string]any)["name"].(string)] = true
		}
		for _, value := range received {
			definition, ok := value.(map[string]any)
			if !ok {
				return invalid()
			}
			tool, ok := definition["tool"].(map[string]any)
			if !ok {
				return invalid()
			}
			name, _ := tool["name"].(string)
			if !requested[name] || definitions[name] != nil || definition["fingerprint"] != expected[name] {
				return invalid()
			}
			bundle := noxmcp.JSON{"tool": tool}
			if cli, ok := definition["cli"]; ok {
				bundle["cli"] = cli
			}
			fp, err := noxmcp.Fingerprint(bundle)
			if err != nil || fp != expected[name] {
				return invalid()
			}
			if state, ok := availability[name]; ok {
				definition["availability"] = state
			}
			definitions[name] = definition
		}
	}
	if len(definitions) != len(expected) {
		return invalid()
	}
	presentation, _ := first["presentation"].(map[string]any)
	ttl, _ := first["ttlMs"].(float64)
	return Catalog{Definitions: definitions, Presentation: presentation, ServerID: serverID, ViewID: viewID, Fingerprint: hash, LoadedAt: time.Now(), TTL: time.Duration(ttl) * time.Millisecond}, nil
}

func validFrame(schema string, value any) bool {
	data, err := json.Marshal(value)
	return err == nil && noxmcp.ValidateFrameWithLimit(schema, data, 8<<20) == nil
}

// CredentialPartition binds in-memory reviews to one configured origin and key,
// including settings replacement by another CLI process. It is never a key copy.
func CredentialPartition(origin, key string) string {
	b, _ := json.Marshal([]string{origin, key})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
