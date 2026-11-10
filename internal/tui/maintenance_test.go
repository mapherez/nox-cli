package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/mapherez/nox-cli/internal/maintenance"
	"github.com/mapherez/nox-cli/internal/setup"
	"strings"
	"testing"
)

func TestUpdateIndicatorAndExplicitConfirmation(t *testing.T) {
	m := newModel(context.Background(), "v1.0.0", setup.New(t.TempDir(), "v1.0.0"))
	updated, _ := m.Update(releaseMsg{check: maintenance.Check{Latest: "v1.1.0"}})
	m = updated.(model)
	if !strings.Contains(m.View().Content, "Update v1.1.0 available") {
		t.Fatal("update indicator missing")
	}
	m.settings = true
	m.maintenancePage = true
	m.maintenanceFocus = 1
	m, cmd := press(m, "enter")
	if !m.confirmUpdate || cmd != nil || m.busy {
		t.Fatal("update executed without confirmation")
	}
	m, cmd = press(m, "esc")
	if m.confirmUpdate || cmd != nil {
		t.Fatal("cancel started update")
	}
	m.maintenanceFocus = 2
	m, _ = press(m, "enter")
	if !strings.Contains(m.View().Content, "Uninstall NoX CLI") {
		t.Fatal("uninstall instructions missing")
	}
	// Business actions own keyboard handling while active, including update keys.
	m.busy = true
	m.busyKind = "tool"
	m.confirmUpdate = false
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Fatal("maintenance ran during a business operation")
	}
}
