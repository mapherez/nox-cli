package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/releases"
)

// Colors follow the terminal background; the CLI does not repaint that background.
// Keep the shared palette here so menus, forms and results can use the same style.
type palette struct {
	accent, secondary, text, muted, rule lipgloss.Style
}

func (m model) palette() palette {
	accent, secondary, text, muted, rule := "#6740B6", "#17796B", "#292C3B", "#5B6170", "#B0B4C0"
	if m.dark {
		accent, secondary, text, muted, rule = "#B7A6FF", "#74D7C5", "#E5E5EF", "#A1A1B5", "#515469"
	}
	style := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	return palette{accent: style(accent), secondary: style(secondary), text: style(text), muted: style(muted), rule: style(rule)}
}

var wordmark = []string{
	"███╗   ██╗       ██╗  ██╗",
	"████╗  ██║ ▄██▄  ╚██╗██╔╝",
	"██╔██╗ ██║██  ██  ╚███╔╝ ",
	"██║╚██╗██║██  ██  ██╔██╗ ",
	"██║ ╚████║ ▀██▀  ██╔╝ ██╗",
	"╚═╝  ╚═══╝       ╚═╝  ╚═╝",
}

func (m model) divider() string {
	return m.palette().rule.Render(strings.Repeat("─", min(m.contentWidth(), 78)))
}

func (m model) header() string {
	p := m.palette()
	version := p.muted.Render(safeText(m.version))
	if releases.Newer(m.latestVersion, m.version) {
		version += p.secondary.Render(" · Update " + safeText(m.latestVersion) + " available")
	}
	if m.settings || m.browser || m.width < 50 || m.height < 24 {
		title := p.accent.Bold(true).Render("NoX") + "  " + version
		if m.width >= 40 && m.height >= 18 {
			return title + "\n" + p.muted.Render("Your Suite, at your fingertips.") + "\n" + m.divider()
		}
		return title
	}
	title := p.accent.Bold(true).Render("Welcome to NoX") + "  " + version
	scene := []string{title, m.divider()}
	moon := []string{
		"      ·          ▄████▄  ",
		"                ███▀     ",
		"   ✦            ██▌    · ",
		"                ███▄     ",
		"        ·        ▀████▀  ",
		"   ▄▄      ▄▄            ",
	}
	for i, line := range wordmark {
		row := p.accent.Bold(true).Render(line)
		if m.contentWidth() >= 66 {
			row += strings.Repeat(" ", max(2, 34-ansi.StringWidth(line)))
			row += p.secondary.Render(moon[i])
		}
		scene = append(scene, row)
	}
	if m.contentWidth() >= 66 && m.height >= 32 {
		scene = append(scene,
			p.rule.Render("                                ▄████▄  ▄████▄       ·"),
			p.rule.Render("          ·                 ▄▄▄████████████████▄▄▄"),
			p.rule.Render("                        ▄▄▄████████████████████████▄▄▄"))
	}
	scene = append(scene, m.divider())
	return strings.Join(scene, "\n")
}

func (m model) menuChoice(label string, selected bool) string {
	p := m.palette()
	if selected {
		return p.accent.Bold(true).Render("> " + label)
	}
	return p.text.Render("  " + label)
}

func (m model) rootChoice(label, description string, selected bool) string {
	row := m.menuChoice(label, selected)
	if m.contentWidth() >= 50 {
		row += strings.Repeat(" ", max(2, 14-len(label))) + m.palette().muted.Render(description)
	}
	return row
}
