package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/credentials"
	"github.com/mapherez/nox-cli/internal/navigation"
)

func (m model) breadcrumb() string {
	parts := []string{"Home"}
	if m.maintenancePage {
		return "Home / Settings / Updates and installation"
	}
	if m.settings {
		return "Home / Settings"
	}
	if !m.browser {
		return "Home"
	}
	parts = append(parts, "Apps")
	if m.tree.Root != nil {
		node := m.tree.Root
		for _, id := range m.route {
			for _, child := range node.Children {
				if child.ID == id {
					node = child
					parts = append(parts, safeText(child.Label))
					break
				}
			}
		}
	}
	if m.details {
		label := "Details"
		if m.activeRun != nil {
			labels := map[string]string{"input": "Input", "pending": "Working", "review": "Review", "done": "Result", "error": "Stopped"}
			if value := labels[m.activeRun.Mode]; value != "" {
				label = value
			}
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " / ")
}
func (m model) paddingX() int     { return min(2, max(0, (m.width-1)/2)) }
func (m model) contentWidth() int { return max(1, m.width-2*m.paddingX()) }
func (m model) menuHeight() int   { return max(1, m.height-lipgloss.Height(m.header())-9) }

// Menus use a blank row between options when the terminal has room.
// Short terminals retain compact rows so selection never disappears.
func (m model) menuStride() int {
	if m.height >= 12 && m.menuHeight() >= 4 {
		return 2
	}
	return 1
}
func (m model) menuPageSize() int { return max(1, m.menuHeight()/m.menuStride()) }

func spaceMenuRows(lines []string, focus int) ([]string, int) {
	spaced := make([]string, 0, len(lines)*2)
	selected := 0
	for i, line := range lines {
		if i == focus {
			selected = len(spaced)
		}
		spaced = append(spaced, line)
		if i+1 < len(lines) && line != "" && lines[i+1] != "" {
			spaced = append(spaced, "")
		}
	}
	return spaced, selected
}

func (m *model) resizeDetails() {
	m.detail.SetWidth(m.contentWidth())
	height := m.menuHeight()
	if m.activeRun != nil && m.activeRun.Mode == "review" {
		if m.activeRun.TypedConfirmation() {
			height -= 2
		}
		if m.formError != "" {
			height -= 2
		}
	}
	m.detail.SetHeight(max(1, height))
}

func (m model) actionText(node *navigation.Node) string {
	a := node.Action
	if a == nil {
		return "No action is attached to this menu."
	}
	lines := []string{safeText(node.Label), "", "Tool: " + safeText(a.Alias)}
	if a.Description != "" {
		lines = append(lines, "", safeText(a.Description))
	}
	if tool, ok := a.Definition["tool"].(map[string]any); ok {
		if schema, ok := tool["inputSchema"].(map[string]any); ok {
			properties, _ := schema["properties"].(map[string]any)
			required, _ := schema["required"].([]any)
			lines = append(lines, "", fmt.Sprintf("Inputs: %d fields, %d required.", len(properties), len(required)))
		}
	}
	if a.Problem != "" {
		lines = append(lines, "", "Unavailable: "+safeText(a.Problem))
	}
	for _, issue := range node.Diagnostics {
		lines = append(lines, safeText(issue))
	}
	lines = append(lines, "", "Enter opens the form and review. Selectors and previews may read data; execution requires review.")
	return strings.Join(lines, "\n")
}

func (m model) settingsLines() ([]string, int) {
	lines := []string{m.origin.View(), m.apiKey.View()}
	if m.apiKey.Value() != "" {
		if err := credentials.ValidateKey(m.apiKey.Value()); err != nil {
			lines = append(lines, err.Error())
		}
	}
	checkbox := "[ ]"
	if m.sessionOnly {
		checkbox = "[x]"
	}
	choices := []string{checkbox + " Use API key only for this session", "Save and test", "Test saved connection", "Remove API key", "Updates and installation", "Back"}
	for i, label := range choices {
		lines = append(lines, m.menuChoice(label, m.focus == i+2))
	}
	if m.confirmRemove {
		lines = append(lines, "", "Remove the saved API key? Enter: Confirm    Esc: Cancel")
	}
	focus := m.focus
	if focus >= 2 && m.apiKey.Value() != "" && credentials.ValidateKey(m.apiKey.Value()) != nil {
		focus++
	}
	if m.confirmRemove {
		focus = len(lines) - 1
	}
	return lines, focus
}

func (m model) browserLines() ([]string, int) {
	if m.busy {
		return []string{"Loading app menus...", "Esc: Cancel"}, 0
	}
	if m.catalog == nil {
		return []string{"No catalog loaded.", "Press R to retry, F2 for Settings, or Esc to go back."}, 0
	}
	if m.details {
		return strings.Split(m.detail.View(), "\n"), 0
	}
	node := m.currentNode()
	rows := m.menuRows()
	lines := []string{}
	if len(rows) == 0 {
		message := "No apps are available to this API key."
		if len(m.route) > 0 {
			message = "This app has no navigable tools."
		}
		if m.search.Value() != "" {
			message = "No matching menu items. Esc clears the search."
		}
		lines = append(lines, message)
	}
	duplicates := map[string]int{}
	for _, row := range rows {
		duplicates[strings.ToLower(safeText(row.Label))]++
	}
	for i, row := range rows {
		label := safeText(row.Label)
		if duplicates[strings.ToLower(label)] > 1 {
			label += " [" + safeText(row.ID) + "]"
		}
		if len(row.Children) > 0 {
			label += " /"
		}
		if row.Action != nil && row.Action.Problem != "" {
			label += " [unavailable]"
		}
		lines = append(lines, m.menuChoice(label, i == m.selected))
	}
	issues := m.tree.Diagnostics
	if node != nil && len(m.route) > 0 {
		issues = node.Diagnostics
	}
	if len(issues) > 0 {
		lines = append(lines, "", fmt.Sprintf("%d menu metadata issue(s). Press ? for details.", len(issues)))
	}
	return lines, m.selected
}

func window(lines []string, selected, height int) []string {
	if len(lines) <= height {
		return lines
	}
	selected = min(max(0, selected), len(lines)-1)
	start := min(max(0, selected-height/2), len(lines)-height)
	return lines[start : start+height]
}

func (m model) View() tea.View {
	width := m.contentWidth()
	header := m.header()
	crumb := m.breadcrumb()
	if ansi.StringWidth(crumb) > width {
		crumb = ansi.TruncateLeft(crumb, ansi.StringWidth(crumb)-width+min(3, width), strings.Repeat(".", min(3, width)))
	}
	var lines []string
	focus := 0
	footer := "Up/Down: Move  Enter: Select  Esc: Exit"
	if m.activeRun != nil {
		lines, focus, footer = m.interactionLines()
	} else if m.maintenancePage {
		lines, focus = m.maintenanceLines()
		footer = "Up/Down: Move  Enter: Select  Esc: Back"
		if m.showUninstall {
			footer = "Up/Down, PgUp/PgDn: Scroll  Esc: Back"
		}
	} else if m.settings {
		lines, focus = m.settingsLines()
		footer = "Tab: Next  Shift+Tab: Previous  Enter: Select  Esc: Back"
	} else if m.browser {
		lines, focus = m.browserLines()
		footer = "Up/Down Move  Enter Open  / Search  R Refresh  ? Info  Esc Back  F2 Settings"
		if m.details {
			footer = "Up/Down, PgUp/PgDn: Scroll  Esc: Back  R: Refresh  F2: Settings"
			if m.detailAction != nil {
				footer = "Enter: Configure / Run  Up/Down: Scroll  Esc: Back  R: Refresh"
			}
		}
		if m.filtering {
			footer = "Type to filter this menu  Enter: Keep filter  Esc: Clear"
		}
	} else {
		for i, item := range [][2]string{{"Apps", "Browse your Suite"}, {"Settings", "Manage your connection"}, {"Exit", "Close NoX"}} {
			lines = append(lines, m.rootChoice(item[0], item[1], i == m.cursor))
		}
		focus = m.cursor
	}
	if (!m.maintenancePage || (!m.showUninstall && !m.confirmUpdate)) && m.activeRun == nil && m.menuStride() > 1 && !m.busy && (m.settings || !m.browser || (m.catalog != nil && !m.details)) {
		lines, focus = spaceMenuRows(lines, focus)
	}
	if m.busy && m.busyKind != "tool" {
		footer = "Esc: Cancel  Ctrl+C: Exit"
	}
	if m.width < 55 && m.activeRun == nil {
		footer = "Enter: Open  Esc: Back  Ctrl+C: Exit"
		if m.settings {
			footer = "Tab: Next  Enter: Select  Esc: Back"
		}
		if m.filtering {
			footer = "Enter: Keep  Esc: Clear"
		}
	}
	p := m.palette()
	search := ""
	if !m.browser && !m.settings {
		search = p.muted.Render("Choose where you want to go.")
	}
	if m.browser && !m.settings && m.activeRun == nil {
		if m.filtering {
			search = m.search.View()
		} else if m.search.Value() != "" {
			search = "Search: " + safeText(m.search.Value())
		} else if m.catalog != nil && !m.details {
			search = fmt.Sprintf("%d item(s)  / to search", len(m.menuRows()))
		}
	}
	if m.height < 12 {
		// Reserve space for the selected row even in a very short terminal.
		body := window(lines, focus, max(1, m.height-4))
		compact := append([]string{crumb}, body...)
		if m.height <= 2 {
			compact = window(lines, focus, m.height)
		}
		if len(compact) < m.height {
			compact = append(compact, m.statusText())
		}
		if len(compact) < m.height {
			compact = append(compact, footer)
		}
		for i, line := range compact {
			compact[i] = ansi.Truncate(line, max(1, m.width), "")
		}
		view := tea.NewView(strings.Join(compact[:min(len(compact), m.height)], "\n"))
		view.AltScreen = true
		return view
	}
	bodyHeight := m.menuHeight()
	lines = window(lines, focus, bodyHeight)
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	content := header + "\n\n" + p.secondary.Bold(true).Render(crumb) + "\n" + search + "\n" + strings.Join(lines, "\n") + "\n" + m.divider() + "\n" + p.muted.Render(m.statusText()) + "\n" + p.muted.Render(footer)
	// Bound every row, including untrusted long labels, without terminal wrapping.
	rendered := strings.Split(content, "\n")
	for i, line := range rendered {
		rendered[i] = ansi.Truncate(line, width, "...")
	}
	if len(rendered) > max(1, m.height-2) {
		rendered = rendered[len(rendered)-max(1, m.height-2):]
	}
	view := tea.NewView(lipgloss.NewStyle().Padding(1, m.paddingX()).Render(strings.Join(rendered, "\n")))
	view.AltScreen = true
	return view
}

func (m model) statusBase() string {
	if m.catalog != nil && !m.busy && strings.HasPrefix(m.status, "Catalog loaded:") {
		count := len(m.tree.Root.Children)
		appLabel := "apps"
		if count == 1 {
			appLabel = "app"
		}
		return fmt.Sprintf("Catalog loaded · %d %s · %d tools · Local navigation", count, appLabel, m.catalog.Report.ToolCount)
	}
	return safeText(m.status)
}
