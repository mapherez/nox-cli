package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/credentials"
)

type message struct{ kind, text string }

func (m *model) setStatus(kind, text string) { m.statusKind = kind; m.status = text }
func (m model) currentMessage() message {
	if m.confirmRemove {
		return message{"warning", "Remove the saved API key? Your Gateway will require a new key."}
	}
	if m.confirmUpdate {
		return message{"warning", "Confirm this update. NoX will close; your settings and saved key will be kept."}
	}
	if m.formError != "" && m.activeRun != nil {
		return message{"error", m.formError}
	}
	if m.settings && !m.maintenancePage && m.apiKey.Value() != "" {
		if err := credentials.ValidateKey(m.apiKey.Value()); err != nil {
			return message{"warning", err.Error()}
		}
	}
	if m.activeRun != nil && !m.busy {
		switch m.activeRun.Mode {
		case "review":
			return message{"info", "Review the action below. The next action runs only after your confirmation."}
		case "input":
			return message{}
		case "error":
			text := m.activeRun.Problem
			if text == "" {
				text = "The action could not be completed. See the result below."
			}
			return message{"error", text}
		case "done":
			return message{}
		}
	}
	return message{m.statusKind, m.status}
}
func (m model) cleanMessage(text string) string {
	text = m.redact(safeParagraph(text))
	if key := m.apiKey.Value(); key != "" {
		text = strings.ReplaceAll(text, key, "[hidden]")
	}
	if m.activeRun != nil {
		if field := m.activeRun.Current(); field != nil && field.Secret {
			for _, value := range []string{m.formValue.Value(), m.formArea.Value()} {
				if value != "" {
					text = strings.ReplaceAll(text, value, "[hidden]")
				}
			}
		}
	}
	return text
}
func (m model) messageStyle(kind string) lipgloss.Style {
	dark, light := "#91B9FF", "#2455A5"
	switch kind {
	case "error":
		dark, light = "#FF9B9B", "#A51F38"
	case "warning":
		dark, light = "#F5CF82", "#805400"
	case "success":
		dark, light = "#80DFC0", "#12634D"
	case "working":
		dark, light = "#91D8F0", "#14647E"
	}
	color := light
	if m.dark {
		color = dark
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
}
func wrapped(text string, width, limit int) []string {
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = ansi.Truncate(lines[limit-1], max(1, width-1), "") + "…"
	}
	return lines
}
func (m model) messageLines(compact bool) []string {
	notice := m.currentMessage()
	if notice.kind == "" || notice.text == "" {
		return nil
	}
	title := strings.ToUpper(notice.kind)
	style := m.messageStyle(notice.kind)
	text := m.cleanMessage(notice.text)
	width := m.contentWidth()
	if compact || width < 16 {
		return []string{style.Render(ansi.Truncate(title+"  "+strings.ReplaceAll(text, "\n", " "), width, "…"))}
	}
	inside := width - 4
	body := wrapped(text, inside, 3)
	label := " " + title + " "
	lines := []string{style.Render("┌" + label + strings.Repeat("─", max(0, width-2-ansi.StringWidth(label))) + "┐")}
	for _, line := range body {
		lines = append(lines, style.Render("│ ")+m.palette().text.Render(line)+strings.Repeat(" ", max(0, inside-ansi.StringWidth(line)))+style.Render(" │"))
	}
	return append(lines, style.Render("└"+strings.Repeat("─", max(0, width-2))+"┘"))
}
func (m model) connectionLine() string {
	state := "Not configured"
	switch {
	case m.busy && (m.busyKind == "browse" || m.busyKind == "save" || m.busyKind == "test"):
		state = "Checking connection…"
	case m.connectionIssue != "":
		state = m.connectionIssue
	case m.lastReport != nil:
		name := strings.TrimSpace(safeText(m.lastReport.ServerName) + " " + safeText(m.lastReport.ServerVersion))
		if name == "" {
			name = "Connection verified"
		}
		state = name + " · Verified this session"
	case m.configured:
		state = "Saved connection · Not checked this session"
	case m.origin.Value() != "":
		state = "API key required"
	}
	return m.palette().muted.Render(ansi.Truncate("GATEWAY  "+state, m.contentWidth(), "…"))
}
func (m model) keyHelp() string {
	if m.busy {
		if m.busyKind == "remove" {
			return "Ctrl+C: Exit"
		}
		if m.busyKind == "tool" {
			return "Esc / Ctrl+C: Cancel"
		}
		return "Esc: Cancel  Ctrl+C: Exit"
	}
	if m.activeRun != nil {
		switch m.activeRun.Mode {
		case "review":
			return "Enter: Confirm / Execute  Ctrl+E: Edit  ↑/↓: Scroll  Esc: Cancel"
		case "done", "error":
			return "Enter / Esc: Back to menu  ↑/↓: Scroll  R: Reload menus"
		case "input":
			if f := m.activeRun.Current(); f != nil {
				switch f.Control {
				case "select", "checkbox":
					return "↑/↓: Move  ←: Previous field  Enter: Next  Ctrl+U: Omit  Esc: Cancel"
				case "multiselect":
					return "↑/↓: Move  ←: Previous field  Space: Toggle  Enter: Next  Esc: Cancel"
				case "textarea", "json":
					return "Arrows: Edit text  Ctrl+D: Next  ← at start: Previous field  Esc: Cancel"
				}
			}
			return "←/→: Edit text  ← at start: Previous field  Enter: Next  Esc: Cancel"
		}
	}
	if m.confirmRemove || m.confirmUpdate {
		return "Enter: Confirm  Esc: Cancel"
	}
	if m.maintenancePage && m.showUninstall {
		return "↑/↓: Scroll  Esc: Back"
	}
	if m.settings {
		return "↑/↓: Move  Enter: Select  Esc: Back"
	}
	if m.filtering {
		return "Type: Filter  Enter: Keep filter  Esc: Clear"
	}
	if m.browser {
		if m.details {
			if m.detailAction != nil {
				return "Enter: Configure / Run  ↑/↓: Scroll  Esc: Back to menu  R: Reload menus"
			}
			return "↑/↓: Scroll  Esc: Back to menu  R: Reload menus"
		}
		return "↑/↓: Move  Enter: Select  ← / Esc: Back  /: Search  R: Refresh  ?: Info  F2: Settings"
	}
	return "↑/↓: Move  Enter: Select  Esc: Exit"
}
func (m model) searchLine() string {
	if m.browser && !m.settings && m.activeRun == nil && !m.details {
		if m.filtering {
			return m.search.View()
		}
		if m.search.Value() != "" {
			return "Search: " + safeText(m.search.Value())
		}
		if m.catalog != nil {
			return fmt.Sprintf("%d menu items", len(m.menuRows()))
		}
	}
	return ""
}

func (m model) breadcrumbLine() string {
	text := m.breadcrumb()
	width := m.contentWidth()
	if ansi.StringWidth(text) > width {
		text = ansi.TruncateLeft(text, ansi.StringWidth(text)-width+min(1, width), "…")
	}
	return m.palette().secondary.Bold(true).Render(text)
}

// chrome reserves separate areas for connection context, messages and key help.
func (m model) chrome() (prefix, suffix []string, body, padding int) {
	width := m.contentWidth()
	help := wrapped("KEYS  "+m.keyHelp(), width, 2)
	for i := range help {
		help[i] = m.palette().muted.Render(help[i])
	}
	if m.height < 16 {
		return m.compactChrome(help)
	}
	padding = 1
	header := m.header()
	build := func(header string) []string {
		lines := strings.Split(header, "\n")
		lines = append(lines, "", m.connectionLine(), m.breadcrumbLine())
		if search := m.searchLine(); search != "" {
			lines = append(lines, search)
		}
		lines = append(lines, "")
		if notice := m.messageLines(false); len(notice) > 0 {
			lines = append(lines, notice...)
			lines = append(lines, "")
		}
		return lines
	}
	prefix = build(header)
	suffix = append([]string{"", m.divider()}, help...)
	body = m.height - 2 - len(prefix) - len(suffix)
	if body < 3 {
		smaller := m
		smaller.browser = true
		prefix = build(smaller.header())
		body = m.height - 2 - len(prefix) - len(suffix)
	}
	if body < 3 {
		smaller := m
		smaller.browser = true
		smaller.height = min(m.height, 17)
		prefix = build(smaller.header())
		suffix = append([]string{""}, help...)
		body = m.height - 2 - len(prefix) - len(suffix)
	}
	if body < 1 {
		return m.compactChrome(help)
	}
	return
}
func (m model) compactChrome(help []string) (prefix, suffix []string, body, padding int) {
	if m.height <= 3 {
		if m.height >= 2 {
			prefix = m.messageLines(true)
		}
		body = max(1, m.height-len(prefix))
		return
	}
	prefix = []string{m.breadcrumbLine()}
	if m.height >= 8 {
		prefix = append(prefix, m.connectionLine())
	}
	prefix = append(prefix, m.messageLines(true)...)
	if m.height >= 6 {
		suffix = help
	}
	body = max(1, m.height-len(prefix)-len(suffix))
	return
}
