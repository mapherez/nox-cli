package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/gateway"
	"github.com/mapherez/nox-cli/internal/interaction"
	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func readyTool(t *testing.T) (model, *atomic.Int32) {
	t.Helper()
	m, _ := browserFixture(t, 1)
	m = loadApps(t, m)
	d := m.catalog.Definitions["alias-000"]
	d["fingerprint"] = strings.Repeat("a", 43)
	tool := d["tool"].(map[string]any)
	tool["_meta"].(map[string]any)["nox/gateway"].(map[string]any)["serverId"] = "test-source"
	tool["annotations"] = map[string]any{"readOnlyHint": false}
	counter := new(atomic.Int32)
	m.manager.Execute = func(_ context.Context, origin, key, version string, call gateway.Call) (*mcp.CallToolResult, error) {
		counter.Add(1)
		if origin != "http://localhost:5711" || key != "test-key" || !call.Revalidate {
			t.Error("incorrect execution boundary")
		}
		return &mcp.CallToolResult{StructuredContent: map[string]any{"id": json.Number("9007199254740993")}, Content: []mcp.Content{&mcp.TextContent{Text: "Actual result\n\x1b]52;c;aGVsbG8=\aSafe output"}}}, nil
	}
	for i := 0; i < 2; i++ {
		m, _ = press(m, "enter")
	}
	// Prepare an unopened action so each test can choose its contract first.
	row := m.menuRows()[0]
	m.route = append(m.route, row.ID)
	m.detailAction = row.Action
	m.detailPushed = true
	m.details = true
	m.detail.SetContent(m.actionText(row))
	return m, counter
}
func settle(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	for i := 0; cmd != nil && i < 30; i++ {
		received := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { received <- c() }(cmd)
		var msg tea.Msg
		select {
		case msg = <-received:
		case <-time.After(5 * time.Second):
			t.Fatal("interaction did not settle")
		}
		updated, next := m.Update(msg)
		m = updated.(model)
		if !m.busy {
			return m
		}
		cmd = next
	}
	return m
}
func TestMutationReviewExecuteResultAndNoAutomaticReplay(t *testing.T) {
	m, calls := readyTool(t)
	if !m.details || calls.Load() != 0 {
		t.Fatal("menu executed a tool")
	}
	m, _ = press(m, "enter")
	if m.activeRun == nil || m.activeRun.Mode != "review" || calls.Load() != 0 {
		t.Fatal("review missing or tool executed before approval", m.status)
	}
	m, cmd := press(m, "enter")
	m = settle(t, m, cmd)
	if m.activeRun.Mode != "done" || calls.Load() != 1 {
		t.Fatal("approved tool did not complete", m.activeRun.Mode)
	}
	view := m.detail.GetContent()
	if !strings.Contains(view, "9007199254740993") || strings.Contains(view, "52;c;") || !strings.Contains(view, "Actual result\nSafe output") {
		t.Fatal("result types or terminal safety lost", view)
	}
	m, cmd = press(m, "enter")
	if cmd != nil || calls.Load() != 1 {
		t.Fatal("result interaction replayed tool")
	}
	if m.activeRun != nil || !m.details {
		t.Fatal("result did not return to tool")
	}
}
func TestTypedConfirmationAcceptsTheLetterEAndKeepsReview(t *testing.T) {
	m, calls := readyTool(t)
	d := m.catalog.Definitions["alias-000"]
	d["tool"].(map[string]any)["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []any{"id"}}
	d["cli"] = map[string]any{"version": "1.0", "confirmation": map[string]any{"mode": "typed", "matchPointer": "/id"}}
	r, e := interaction.New(*m.catalog, "alias-000")
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Set("delete-me", true); e != nil {
		t.Fatal(e)
	}
	m.activeRun = r
	_ = m.syncInteraction()
	for _, ch := range "delete-me" {
		m, _ = press(m, string(ch))
	}
	if m.confirmValue.Value() != "delete-me" || r.Mode != "review" || calls.Load() != 0 {
		t.Fatal("typing e edited the form or bypassed review")
	}
	m, cmd := press(m, "enter")
	m = settle(t, m, cmd)
	if calls.Load() != 1 || m.activeRun.Mode != "done" {
		t.Fatal("typed approval not executed")
	}
}
func TestPasswordIsMaskedInFormReviewAndReturnedContent(t *testing.T) {
	m, _ := readyTool(t)
	d := m.catalog.Definitions["alias-000"]
	d["tool"].(map[string]any)["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"password": map[string]any{"type": "string"}}, "required": []any{"password"}}
	d["cli"] = map[string]any{"version": "1.0", "parameters": map[string]any{"/password": map[string]any{"control": "password"}}}
	m, _ = press(m, "enter")
	secret := "private-test-password"
	for _, ch := range secret {
		m, _ = press(m, string(ch))
	}
	if strings.Contains(m.View().Content, secret) {
		t.Fatal("password visible while typing")
	}
	m, _ = press(m, "enter")
	if m.activeRun.Mode != "review" || strings.Contains(m.detail.GetContent(), secret) {
		t.Fatal("password visible in review")
	}
	m.manager.Execute = func(context.Context, string, string, string, gateway.Call) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: secret}}}, nil
	}
	m, cmd := press(m, "enter")
	m = settle(t, m, cmd)
	if strings.Contains(m.detail.GetContent(), secret) {
		t.Fatal("password echoed in results")
	}
}
func TestExecutionRejectionDropsFormsAndStaleReviewCannotReplay(t *testing.T) {
	for _, err := range []error{gateway.ErrRejected, gateway.ErrStale, gateway.ErrAccess} {
		m, _ := readyTool(t)
		m.manager.Execute = func(context.Context, string, string, string, gateway.Call) (*mcp.CallToolResult, error) {
			return nil, err
		}
		m, _ = press(m, "enter")
		m, cmd := press(m, "enter")
		m = settle(t, m, cmd)
		if m.catalog != nil {
			t.Fatal("invalid view survived", err)
		}
		if errors.Is(err, gateway.ErrRejected) {
			if m.activeRun != nil || !m.settings || m.configured || len(m.route) != 0 {
				t.Fatal("credential rejection retained state")
			}
		} else {
			if m.activeRun == nil || m.activeRun.Mode != "error" || !m.configured {
				t.Fatal("permission failure confused with credential rejection")
			}
			_, cmd = press(m, "enter")
			if cmd != nil {
				t.Fatal("stale action replayed")
			}
		}
	}
}
func TestInteractionResizesAndOptionalJSONControlsPreserveTypes(t *testing.T) {
	m, _ := readyTool(t)
	d := m.catalog.Definitions["alias-000"]
	d["tool"].(map[string]any)["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}}, "required": []any{"items"}}
	d["cli"] = noxmcp.JSON{"version": "1.0"}
	m, _ = press(m, "enter")
	m.formArea.SetValue("[1,9007199254740993]")
	m, _ = press(m, "ctrl+d")
	if m.activeRun.Mode != "review" || m.activeRun.Arguments["items"].([]any)[1] != json.Number("9007199254740993") {
		t.Fatal("JSON editor changed types")
	}
	for _, size := range [][2]int{{80, 24}, {30, 12}, {9, 5}, {1, 1}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(model)
		view := m.View().Content
		if lipgloss.Height(view) > size[1] {
			t.Fatal("interaction exceeds height", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("interaction exceeds width", size)
			}
		}
	}
}

func TestCheckboxMultiselectAndTextareaControls(t *testing.T) {
	m, _ := readyTool(t)
	d := m.catalog.Definitions["alias-000"]
	d["tool"].(map[string]any)["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"flag": map[string]any{"type": "boolean"}, "values": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []any{"a", "b"}}}, "note": map[string]any{"type": "string"}}, "required": []any{"flag", "values", "note"}}
	d["cli"] = map[string]any{"version": "1.0", "parameters": map[string]any{"/flag": map[string]any{"order": float64(1)}, "/values": map[string]any{"control": "multiselect", "order": float64(2)}, "/note": map[string]any{"control": "textarea", "order": float64(3)}}}
	m, _ = press(m, "enter")
	m, _ = press(m, "down")
	m, _ = press(m, "enter")
	if m.activeRun.Current().Pointer != "/values" {
		t.Fatal("checkbox did not advance")
	}
	m, _ = press(m, "space")
	m, _ = press(m, "enter")
	if m.activeRun.Current().Control != "textarea" {
		t.Fatal("multiselect did not advance")
	}
	m.formArea.SetValue("line one\nline two")
	m, _ = press(m, "ctrl+d")
	args := m.activeRun.Arguments
	if m.activeRun.Mode != "review" || args["flag"] != false || args["values"].([]any)[0] != "a" || args["note"] != "line one\nline two" {
		t.Fatal("controls changed JSON types", args)
	}
}

func TestOptionalOmissionAndOversizedPasteNeverInventValues(t *testing.T) {
	m, _ := readyTool(t)
	d := m.catalog.Definitions["alias-000"]
	d["tool"].(map[string]any)["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"note": map[string]any{"type": "string", "default": "do-not-insert"}}}
	m, _ = press(m, "enter")
	updated, _ := m.Update(tea.PasteMsg{Content: strings.Repeat("a", (1<<20)+1)})
	m = updated.(model)
	if m.formValue.Value() != "" || m.formError == "" {
		t.Fatal("oversized paste silently truncated")
	}
	m, _ = press(m, "ctrl+u")
	if m.activeRun.Mode != "review" || len(m.activeRun.Arguments) != 0 {
		t.Fatal("omission inserted an empty value or default")
	}
}
func TestCtrlCDuringExecutionCancelsAndDisplaysUnknownWithoutExiting(t *testing.T) {
	m, calls := readyTool(t)
	m.catalog.Definitions["alias-000"]["tool"].(map[string]any)["annotations"] = map[string]any{"readOnlyHint": false}
	m.manager.Execute = func(ctx context.Context, _, _, _ string, c gateway.Call) (*mcp.CallToolResult, error) {
		calls.Add(1)
		c.Progress("Queued")
		<-ctx.Done()
		return nil, gateway.ErrUnknown
	}
	m, _ = press(m, "enter")
	m, cmd := press(m, "enter")
	updated, quit := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	m = updated.(model)
	if quit != nil || !m.busy {
		t.Fatal("Ctrl+C exited instead of canceling")
	}
	m = settle(t, m, cmd)
	if m.activeRun.Mode != "error" || !strings.Contains(m.detail.GetContent(), "outcome is unknown") || calls.Load() != 1 {
		t.Fatal("cancellation hid an unknown write or replayed it")
	}
}
