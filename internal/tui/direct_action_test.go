package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mapherez/nox-cli/internal/gateway"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOneEnterRunsParameterFreeReadAndReturnsToTheMenu(t *testing.T) {
	m, calls := readyTool(t)
	m.catalog.Definitions["alias-000"]["tool"].(map[string]any)["annotations"] = map[string]any{"readOnlyHint": true, "destructiveHint": false}
	m, _ = press(m, "esc") // Start at the real parent menu, not a synthetic detail view.
	parent := m.breadcrumb()
	m, _ = press(m, "right")
	if m.busy || calls.Load() != 0 || m.breadcrumb() != parent {
		t.Fatal("navigation arrow ran an action")
	}
	m, cmd := press(m, "enter")
	if !m.busy || m.activeRun == nil || m.activeRun.Mode != "pending" || cmd == nil {
		t.Fatal("Enter did not directly request the read", m.status)
	}
	m = settle(t, m, cmd)
	if calls.Load() != 1 || m.activeRun.Mode != "done" || strings.Contains(m.breadcrumb(), "Review") {
		t.Fatal("read required additional menus or ran more than once")
	}
	m, cmd = press(m, "enter")
	if cmd != nil || calls.Load() != 1 {
		t.Fatal("Enter on the result replayed the call")
	}
	if m.activeRun != nil || m.details || m.breadcrumb() != parent {
		t.Fatal("result did not return to its parent menu", m.breadcrumb())
	}
}
func TestReadWithPublishedConfirmationStillRequiresIt(t *testing.T) {
	m, calls := readyTool(t)
	definition := m.catalog.Definitions["alias-000"]
	definition["tool"].(map[string]any)["annotations"] = map[string]any{"readOnlyHint": true}
	definition["cli"] = map[string]any{"version": "1.0", "confirmation": map[string]any{"mode": "boolean", "message": "Confirm this read."}}
	m, _ = press(m, "esc")
	m, cmd := press(m, "enter")
	if m.activeRun == nil || m.activeRun.Mode != "review" || m.busy || calls.Load() != 0 || cmd != nil {
		t.Fatal("explicit confirmation was skipped")
	}
}
func TestReadableJSONResultDeduplicatesOnlyEquivalentContent(t *testing.T) {
	m, _ := readyTool(t)
	m.manager.Execute = func(context.Context, string, string, string, gateway.Call) (*mcp.CallToolResult, error) {
		data := map[string]any{"version": "v3.2.1", "status": "ready", "dataInitialized": true, "revision": json.Number("9007199254740993")}
		raw, _ := json.Marshal(data)
		return &mcp.CallToolResult{StructuredContent: data, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}, &mcp.TextContent{Text: "Additional useful information."}}}, nil
	}
	m.catalog.Definitions["alias-000"]["tool"].(map[string]any)["annotations"] = map[string]any{"readOnlyHint": true}
	m, _ = press(m, "esc")
	m, cmd := press(m, "enter")
	m = settle(t, m, cmd)
	text := m.detail.GetContent()
	for _, wanted := range []string{"Version: v3.2.1", "Status: ready", "Data Initialized: Yes", "Revision: 9007199254740993", "Additional useful information."} {
		if !strings.Contains(text, wanted) {
			t.Fatal("readable field missing", wanted, text)
		}
	}
	if strings.Count(text, "9007199254740993") != 1 || strings.Contains(text, "Structured result") || strings.Contains(text, `"version"`) {
		t.Fatal("raw or duplicate JSON was displayed", text)
	}
}
func TestReadableResultsHandleNestedObjectsArraysAndFalseValues(t *testing.T) {
	data := map[string]any{"group": map[string]any{"enabled": false, "missing": nil}, "rows": []any{map[string]any{"display_name": "Alpha", "id": "opaque-1"}, map[string]any{"display_name": "Beta", "id": "opaque-2"}}, "empty": []any{}}
	output := readableResult(data, nil)
	for _, wanted := range []string{"Group", "Enabled: No", "Missing: Not set", "Display Name: Alpha", "ID: opaque-2", "No items."} {
		if !strings.Contains(output, wanted) {
			t.Fatal("nested result missing", wanted, output)
		}
	}
}

// Deliberately uses arbitrary tool/field names: the flow must be driven by the
// published selector contract and read annotation, not a product-specific path.
func TestSelectedTargetRunsReadWithoutRequestingAnIDOrReview(t *testing.T) {
	m, _ := readyTool(t)
	d := m.catalog.Definitions["alias-000"]
	tool := d["tool"].(map[string]any)
	tool["annotations"] = map[string]any{"readOnlyHint": true}
	tool["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"target": map[string]any{"type": "string", "title": "Choose a target"}}, "required": []any{"target"}, "additionalProperties": false}
	d["cli"] = map[string]any{"version": "1.0", "parameters": map[string]any{"/target": map[string]any{"options": map[string]any{"tool": "source-choices", "itemsPointer": "/items", "valuePointer": "/key", "labelPointer": "/label"}}}}
	m.catalog.Definitions["source-choices"] = map[string]any{"fingerprint": strings.Repeat("b", 43), "tool": map[string]any{"name": "source-choices", "_meta": tool["_meta"], "annotations": map[string]any{"readOnlyHint": true}, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}}}
	lookups, executions := 0, 0
	m.manager.Execute = func(_ context.Context, _, _, _ string, call gateway.Call) (*mcp.CallToolResult, error) {
		if call.Name == "source-choices" {
			lookups++
			return &mcp.CallToolResult{StructuredContent: map[string]any{"items": []any{map[string]any{"key": "opaque-1", "label": "Alpha"}, map[string]any{"key": "opaque-2", "label": "Beta"}}}}, nil
		}
		executions++
		if call.Name != "alias-000" || call.Arguments["target"] != "opaque-2" || len(call.Arguments) != 1 || !call.Revalidate {
			t.Error("selected target was lost or dispatch validation was skipped", call)
		}
		return &mcp.CallToolResult{StructuredContent: map[string]any{"status": "ready"}}, nil
	}
	m, _ = press(m, "esc")
	parent := m.breadcrumb()
	m, cmd := press(m, "enter")
	m = settle(t, m, cmd)
	if m.activeRun.Mode != "input" || len(m.currentOptions()) != 2 || executions != 0 {
		t.Fatal("published target selector was skipped")
	}
	m, _ = press(m, "down")
	m, cmd = press(m, "enter")
	if m.activeRun.Mode != "pending" || !m.busy || cmd == nil {
		t.Fatal("target selection opened another form or review instead of running")
	}
	m = settle(t, m, cmd)
	if lookups != 1 || executions != 1 || m.activeRun.Mode != "done" {
		t.Fatal("query or selector was repeated")
	}
	if notice := m.currentMessage(); notice.kind != "" || notice.text != "" {
		t.Fatal("ordinary result has a completion banner", notice)
	}
	if !strings.Contains(m.keyHelp(), "Enter / Esc: Back to menu") {
		t.Fatal("result does not explain how to return")
	}
	m, cmd = press(m, "enter")
	if m.activeRun != nil || m.breadcrumb() != parent || cmd != nil || executions != 1 || m.status != "" {
		t.Fatal("Enter did not return quietly without replaying")
	}
}
