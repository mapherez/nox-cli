package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mapherez/nox-cli/internal/maintenance"
	"github.com/mapherez/nox-cli/internal/releases"
)

type releaseMsg struct {
	check maintenance.Check
	err   error
}
type updatedMsg struct {
	restart bool
	err     error
}

func (m model) checkRelease(force bool) tea.Cmd {
	return func() tea.Msg { check, err := m.updater.Check(m.ctx, force); return releaseMsg{check: check, err: err} }
}
func (m *model) applyUpdate() tea.Cmd {
	m.busy = true
	m.busyKind = "update"
	m.status = "Downloading and verifying the update..."
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	version := m.latestVersion
	updater := m.updater
	return func() tea.Msg {
		defer cancel()
		restart, err := updater.Update(ctx, version)
		return updatedMsg{restart: restart, err: err}
	}
}
func (m model) maintenanceKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if m.showUninstall {
		switch k {
		case "esc", "enter":
			m.showUninstall = false
		case "up", "k":
			m.maintenanceScroll = max(0, m.maintenanceScroll-1)
		case "down", "j":
			m.maintenanceScroll = min(16, m.maintenanceScroll+1)
		case "pgup":
			m.maintenanceScroll = max(0, m.maintenanceScroll-m.menuHeight())
		case "pgdown":
			m.maintenanceScroll = min(16, m.maintenanceScroll+m.menuHeight())
		}
		return m, nil
	}
	if m.confirmUpdate {
		if k == "esc" {
			m.confirmUpdate = false
			return m, nil
		}
		if k == "enter" {
			m.confirmUpdate = false
			return m, m.applyUpdate()
		}
		return m, nil
	}
	switch k {
	case "esc":
		m.maintenancePage = false
		m.focus = 6
		return m, nil
	case "up", "k", "shift+tab":
		m.maintenanceFocus = (m.maintenanceFocus + 3) % 4
	case "down", "j", "tab":
		m.maintenanceFocus = (m.maintenanceFocus + 1) % 4
	case "enter":
		switch m.maintenanceFocus {
		case 0:
			if !m.releaseChecking {
				m.releaseChecking = true
				m.releaseStatus = "Checking official releases..."
				return m, m.checkRelease(true)
			}
		case 1:
			if releases.Newer(m.latestVersion, m.version) {
				m.confirmUpdate = true
			} else {
				m.releaseStatus = "No newer release is ready to install. Check for updates first."
			}
		case 2:
			m.showUninstall = true
			m.maintenanceScroll = 0
		case 3:
			m.maintenancePage = false
			m.focus = 6
		}
	}
	return m, nil
}
func (m model) maintenanceLines() ([]string, int) {
	if m.showUninstall {
		return []string{"Uninstall NoX CLI", "", "Keep settings and the stored API key:", "macOS / Linux:", "curl -fsSL https://cli.mapherez.com/uninstall.sh | sh", "Windows PowerShell:", "$script = irm https://cli.mapherez.com/uninstall.ps1", "& ([scriptblock]::Create($script))", "", "To also delete settings and the single owned API key:", "macOS / Linux: append -s -- --purge after sh.", "Windows: append -Purge after the final parenthesis.", "", "Only installer-owned files and PATH changes are removed.", "Unrelated files are kept. Close other NoX CLI processes first.", "", "Esc: Back"}, m.maintenanceScroll
	}
	if m.confirmUpdate {
		return []string{"Install NoX CLI " + safeText(m.latestVersion) + "?", "", "NoX will close after the update is prepared.", "Run nox again to use the updated version.", "Your configuration and saved API key are preserved.", "", "Enter: Update    Esc: Cancel"}, 6
	}
	status := m.releaseStatus
	if status == "" {
		status = "Check for an official release. Source builds are not replaced."
	}
	target := "Install update"
	if m.latestVersion != "" {
		target += " (" + safeText(m.latestVersion) + ")"
	}
	lines := []string{fmt.Sprintf("Installed version: %s", safeText(m.version)), safeText(status), ""}
	for i, label := range []string{"Check for updates", target, "Uninstall instructions", "Back"} {
		lines = append(lines, m.menuChoice(label, m.maintenanceFocus == i))
	}
	return lines, 3 + m.maintenanceFocus
}
func (m model) updateStatus() string {
	if releases.Newer(m.latestVersion, m.version) {
		return "Update " + safeText(m.latestVersion) + " available · Settings → Updates and installation"
	}
	return ""
}
func (m model) statusText() string {
	text := m.statusBase()
	if update := m.updateStatus(); update != "" {
		text = strings.TrimSpace(text) + " | " + update
	}
	return text
}
