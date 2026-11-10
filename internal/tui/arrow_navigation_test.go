package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mapherez/nox-cli/internal/interaction"
	"github.com/mapherez/nox-cli/internal/setup"
)

func TestSettingsArrowNavigationPreservesInputsAndNeverExecutesActions(t *testing.T) {
	m := newModel(context.Background(), "dev", setup.New(t.TempDir(), "dev"))
	_ = m.openSettings()
	m.origin.SetValue("http://private.example:5711")
	m.apiKey.SetValue("masked-draft")
	for i := 1; i <= 8; i++ {
		var cmd tea.Cmd
		m, cmd = press(m, "down")
		if m.focus != i%8 || m.busy || m.confirmRemove || m.maintenancePage {
			t.Fatalf("Down executed an action or lost focus: %d", m.focus)
		}
		// Focus commands only control the caret; do not run a connection operation.
		_ = cmd
	}
	m, _ = press(m, "up")
	if m.focus != 7 {
		t.Fatal("Up did not wrap to the previous item")
	}
	if m.origin.Value() != "http://private.example:5711" || m.apiKey.Value() != "masked-draft" {
		t.Fatal("navigation lost a draft")
	}
	for _, size := range [][2]int{{80, 24}, {45, 18}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(model)
		view := m.View().Content
		if strings.Contains(view, "Tab:") || !strings.Contains(view, "↑/↓:") || strings.Contains(view, "masked-draft") {
			t.Fatal("incorrect help or exposed key")
		}
	}
	m.focus = 6
	m, _ = press(m, "enter")
	if !m.maintenancePage {
		t.Fatal("Enter did not open maintenance")
	}
	for i := 1; i <= 4; i++ {
		m, _ = press(m, "down")
		if m.maintenanceFocus != i%4 || m.busy || m.confirmUpdate || m.showUninstall {
			t.Fatal("arrow executed a maintenance action")
		}
	}
	m, _ = press(m, "up")
	if m.maintenanceFocus != 3 {
		t.Fatal("maintenance Up did not move")
	}
	m, _ = press(m, "esc")
	if m.maintenancePage || !m.settings || m.focus != 6 {
		t.Fatal("Esc did not return to Settings")
	}
}

func TestFormBackArrowPreservesTextCursorBehavior(t *testing.T) {
	m, _ := readyTool(t)
	definition := m.catalog.Definitions["alias-000"]
	definition["tool"].(map[string]any)["inputSchema"] = map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}, "b": map[string]any{"type": "string"}}, "required": []any{"a", "b"}}
	run, err := interaction.New(*m.catalog, "alias-000")
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Set("first", true); err != nil {
		t.Fatal(err)
	}
	m.activeRun = run
	_ = m.syncInteraction()
	m.formValue.SetValue("edit")
	for i := 0; i < 4; i++ {
		m, _ = press(m, "left")
		if m.activeRun.Index != 1 {
			t.Fatal("Left inside text unexpectedly changed field")
		}
	}
	m, _ = press(m, "left")
	if m.activeRun.Index != 0 {
		t.Fatal("Left at the start did not return to the previous field")
	}
	footer := m.keyHelp()
	if strings.Contains(footer, "Tab") {
		t.Fatal("form still requires Tab navigation")
	}
}
