package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/gateway"
	"github.com/mapherez/nox-cli/internal/maintenance"
	"github.com/mapherez/nox-cli/internal/setup"
)

func TestMessagesAreSeparateFromGatewayAndKeys(t *testing.T) {
	m := newModel(context.Background(), "dev", setup.New(t.TempDir(), "dev"))
	m.configured = true
	report := gateway.Report{ServerName: "NoX Gateway", ServerVersion: "v0.0.1"}
	m.lastReport = &report
	for _, kind := range []string{"error", "warning", "success", "working"} {
		m.setStatus(kind, "A clear operation message.")
		view := ansi.Strip(m.View().Content)
		gatewayLine, notice, keys := strings.Index(view, "GATEWAY"), strings.Index(view, strings.ToUpper(kind)), strings.Index(view, "KEYS")
		if gatewayLine < 0 || notice <= gatewayLine || keys <= notice {
			t.Fatalf("message areas are not separated: %s", view)
		}
		if !strings.Contains(view, "A clear operation message.") {
			t.Fatal("message was cut off")
		}
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "GATEWAY") && (strings.Contains(line, "KEYS") || strings.Contains(line, "A clear")) {
				t.Fatal("status and help are mixed")
			}
		}
	}
}
func TestConnectionFailureAndSuccessUseExplicitMessageKinds(t *testing.T) {
	m := newModel(context.Background(), "dev", setup.New(t.TempDir(), "dev"))
	updated, _ := m.Update(checkedMsg{kind: "test", err: errors.New("The Gateway could not be reached.")})
	m = updated.(model)
	if m.currentMessage().kind != "error" || m.lastReport != nil {
		t.Fatal("failed check claimed success")
	}
	updated, _ = m.Update(checkedMsg{kind: "test", report: gateway.Report{ServerName: "Gateway", ServerVersion: "1"}})
	m = updated.(model)
	if m.currentMessage().kind != "success" || m.lastReport == nil || strings.Contains(m.status, "MCP") {
		t.Fatal("server metadata was mixed into success message")
	}
	m.maintenancePage = true
	m.releaseChecking = true
	updated, _ = m.Update(releaseMsg{check: maintenance.Check{}, err: errors.New("offline")})
	m = updated.(model)
	if m.currentMessage().kind != "warning" || m.lastReport == nil {
		t.Fatal("release outage incorrectly changed the Gateway connection")
	}
}
func TestMessageWrappingSafetyAndSmallTerminalSelection(t *testing.T) {
	m, _ := browserFixture(t, 80)
	m = loadApps(t, m)
	m, _ = press(m, "enter")
	m, _ = press(m, "enter")
	m, _ = press(m, "end")
	secret := "typed-private-key"
	m.apiKey.SetValue(secret)
	m.setStatus("error", strings.Repeat("A long but useful diagnostic. ", 20)+secret+"\x1b]52;c;leak\a")
	for _, size := range [][2]int{{90, 28}, {45, 24}, {26, 12}, {9, 5}, {1, 1}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(model)
		view := m.View().Content
		if lipgloss.Height(view) > size[1] {
			t.Fatal("message exceeded terminal height", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("message exceeded terminal width", size)
			}
		}
		if strings.Contains(view, secret) || strings.Contains(view, "52;c;") {
			t.Fatal("message exposed secret or terminal controls")
		}
		if size[0] > 20 && !strings.Contains(view, "Action 079") {
			t.Fatal("message hid the selected item", size, view)
		}
	}
}
func TestConfirmationTakesPriorityOverDraftValidation(t *testing.T) {
	m := newModel(context.Background(), "dev", setup.New(t.TempDir(), "dev"))
	m.settings = true
	m.apiKey.SetValue("invalid")
	m.confirmRemove = true
	if !strings.Contains(m.currentMessage().text, "Remove the saved API key?") {
		t.Fatal("confirmation hidden by draft validation")
	}
}
