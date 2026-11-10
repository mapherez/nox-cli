package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
func (m model) menuHeight() int   { _, _, body, _ := m.chrome(); return body }

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
	lines = append(lines, "", "Selectors and previews may read data. Execution requires review.")
	return strings.Join(lines, "\n")
}

func (m model) settingsLines() ([]string, int) {
	lines := []string{m.origin.View(), m.apiKey.View()}

	checkbox := "[ ]"
	if m.sessionOnly {
		checkbox = "[x]"
	}
	choices := []string{checkbox + " Use API key only for this session", "Save and test", "Test saved connection", "Remove API key", "Updates and installation", "Back"}
	for i, label := range choices {
		lines = append(lines, m.menuChoice(label, m.focus == i+2))
	}
	focus := m.focus
	return lines, focus
}

func (m model) browserLines() ([]string, int) {
	if m.busy {
		return []string{"Loading app menus…"}, 0
	}
	if m.catalog == nil {
		return []string{"No catalog loaded."}, 0
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
			message = "No matching menu items."
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
		lines = append(lines, "", fmt.Sprintf("%d menu metadata issue(s).", len(issues)))
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
	prefix, suffix, height, padding := m.chrome()
	// Keep scrollable detail content within the space left by messages/help.
	m.detail.SetWidth(m.contentWidth())
	detailHeight := height
	if m.activeRun != nil && m.activeRun.Mode == "review" && m.activeRun.TypedConfirmation() {
		detailHeight -= 2
	}
	m.detail.SetHeight(max(1, detailHeight))
	var lines []string
	focus := 0
	switch {
	case m.activeRun != nil:
		lines, focus = m.interactionLines()
	case m.maintenancePage:
		lines, focus = m.maintenanceLines()
	case m.settings:
		lines, focus = m.settingsLines()
	case m.browser:
		lines, focus = m.browserLines()
	default:
		for i, item := range [][2]string{{"Apps", "Browse your Suite"}, {"Settings", "Manage your connection"}, {"Exit", "Close NoX"}} {
			lines = append(lines, m.rootChoice(item[0], item[1], i == m.cursor))
		}
		focus = m.cursor
	}
	if (!m.maintenancePage || (!m.showUninstall && !m.confirmUpdate)) && m.activeRun == nil && m.menuStride() > 1 && !m.busy && (m.settings || !m.browser || (m.catalog != nil && !m.details)) {
		lines, focus = spaceMenuRows(lines, focus)
	}
	lines = window(lines, focus, height)
	for len(lines) < height {
		lines = append(lines, "")
	}
	rendered := append(append(prefix, lines...), suffix...)
	for i, line := range rendered {
		rendered[i] = ansi.Truncate(line, m.contentWidth(), "…")
	}
	content := strings.Join(rendered, "\n")
	view := tea.NewView(lipgloss.NewStyle().Padding(padding, m.paddingX()).Render(content))
	view.AltScreen = true
	return view
}
