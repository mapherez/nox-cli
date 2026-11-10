// Package tui owns terminal state and rendering, not credential persistence.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/config"
	"github.com/mapherez/nox-cli/internal/credentials"
	"github.com/mapherez/nox-cli/internal/gateway"
	"github.com/mapherez/nox-cli/internal/interaction"
	"github.com/mapherez/nox-cli/internal/maintenance"
	"github.com/mapherez/nox-cli/internal/navigation"
	"github.com/mapherez/nox-cli/internal/releases"
	"github.com/mapherez/nox-cli/internal/setup"
)

type checkedMsg struct {
	generation int
	kind       string
	report     gateway.Report
	catalog    *gateway.Catalog
	err        error
	usingSaved bool
}

type model struct {
	maintenanceScroll                                              int
	updater                                                        *maintenance.Manager
	maintenancePage, showUninstall, confirmUpdate, releaseChecking bool
	maintenanceFocus                                               int
	latestVersion, releaseStatus, exitNotice                       string

	activeRun               *interaction.Run
	detailAction            *navigation.Action
	formValue, confirmValue textinput.Model
	formArea                textarea.Model
	formError               string
	optionCursor            int
	optionSelected          map[int]bool
	callEvents              <-chan tea.Msg

	version                               string
	width, height                         int
	dark                                  bool
	manager                               *setup.Manager
	ctx                                   context.Context
	origin, apiKey, search                textinput.Model
	settings, browser, details, filtering bool
	detailPushed                          bool
	settingsReturn                        bool
	cursor, focus, selected               int
	sessionOnly, configured, busy         bool
	busyKind                              string
	confirmRemove                         bool
	cancel                                context.CancelFunc
	generation                            int
	status                                string
	catalog                               *gateway.Catalog
	tree                                  navigation.Tree
	route                                 []string
	detail                                viewport.Model
}

func newModel(ctx context.Context, version string, manager *setup.Manager) model {
	address, secret, search := textinput.New(), textinput.New(), textinput.New()
	address.Placeholder = "http://gateway.home.arpa:5711"
	address.Prompt = "Gateway: "
	address.CharLimit = 2048
	secret.Placeholder = "Enter a new key, or leave blank to keep the saved key"
	secret.Prompt = "API key: "
	secret.EchoMode = textinput.EchoPassword
	secret.CharLimit = 4096
	search.Prompt = "Search: "
	search.Placeholder = "Filter this menu"
	search.CharLimit = 120
	m := model{ctx: ctx, version: version, updater: maintenance.New(manager.Store.Root, version), width: 80, height: 24, dark: true, manager: manager, origin: address, apiKey: secret, search: search,
		formValue: textinput.New(), confirmValue: textinput.New(), formArea: textarea.New(),
		cursor: 1, status: "Not configured", detail: viewport.New()}
	m.detail.SoftWrap = true
	settings, err := manager.Settings()
	if err != nil {
		m.status = err.Error()
	} else if settings.GatewayOrigin != "" {
		m.origin.SetValue(settings.GatewayOrigin)
		m.sessionOnly = settings.SessionOnly
		if settings.SessionOnly {
			m.status = "Session-only mode: enter an API key for this session."
		} else if settings.CredentialRef != "" {
			m.configured = true
			m.cursor = 0
			m.status = "API key configured. Open Apps to load the catalog."
		}
	}
	if notice := m.updater.Result(); notice != "" {
		m.status += " " + safeText(notice)
	}
	return m
}

func (m model) Init() tea.Cmd {
	if releases.Version(m.version) {
		return tea.Batch(tea.RequestBackgroundColor, m.checkRelease(false))
	}
	return tea.RequestBackgroundColor
}
func (m *model) focusInput() tea.Cmd {
	m.origin.Blur()
	m.apiKey.Blur()
	if m.focus == 0 {
		return m.origin.Focus()
	}
	if m.focus == 1 {
		return m.apiKey.Focus()
	}
	return nil
}
func (m *model) openSettings() tea.Cmd {
	m.settingsReturn = m.browser
	m.filtering = false
	m.search.Blur()
	m.settings = true
	m.focus = 0
	m.confirmRemove = false
	return m.focusInput()
}
func (m *model) closeSettings() {
	m.settings = false
	m.browser = m.settingsReturn && m.catalog != nil
	m.apiKey.SetValue("")
	m.origin.Blur()
	m.apiKey.Blur()
}
func (m *model) clearCatalog() {
	m.catalog = nil
	m.route = nil
	m.detailPushed = false
	m.detailAction = nil
	m.tree = navigation.Tree{}
	m.details = false
	m.selected = 0
}

func (m *model) operation(kind string) tea.Cmd {
	m.busy = true
	m.busyKind = kind
	m.status = "Checking connection..."
	if kind == "browse" {
		m.status = "Loading app menus from Gateway..."
	} else {
		m.clearCatalog() // A connection/key change cannot retain another view's menu.
		if kind == "remove" {
			m.configured = false
			m.status = "Removing API key..."
		}
	}
	m.generation++
	generation := m.generation
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	manager, origin, apiKey, sessionOnly := m.manager, m.origin.Value(), m.apiKey.Value(), m.sessionOnly
	return func() tea.Msg {
		defer cancel()
		result := checkedMsg{generation: generation, kind: kind, usingSaved: apiKey == ""}
		switch kind {
		case "browse":
			catalog, err := manager.Browse(ctx)
			result.catalog = &catalog
			result.err = err
			if ctx.Err() != nil {
				result.catalog = nil
				result.err = ctx.Err()
			}
		case "save":
			result.report, result.err = manager.Save(ctx, origin, apiKey, sessionOnly)
		case "test":
			result.report, result.err = manager.Check(ctx)
		case "remove":
			result.err = manager.Remove()
		}
		return result
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case releaseMsg:
		m.releaseChecking = false
		m.latestVersion = msg.check.Latest
		if msg.err != nil {
			m.releaseStatus = "Update check unavailable. Suite access is unaffected; try again later."
		} else if !releases.Version(m.version) && m.latestVersion != "" {
			m.releaseStatus = "Latest stable release: " + m.latestVersion + ". Source builds are not replaced."
		} else if releases.Newer(m.latestVersion, m.version) {
			m.releaseStatus = "Update " + m.latestVersion + " is available."
		} else {
			m.releaseStatus = "No newer stable release is available."
		}
		return m, nil
	case updatedMsg:
		m.busy = false
		m.cancel = nil
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.exitNotice = "NoX CLI update prepared. Run nox again to use the updated version."
		m.apiKey.SetValue("")
		return m, tea.Quit

	case callProgress, callFinished:
		return m.interactionEvent(msg)
	case checkedMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.busy = false
		m.cancel = nil
		if msg.err != nil {
			m.status = msg.err.Error()
			if msg.kind == "browse" {
				m.clearCatalog()
			}
			if errors.Is(msg.err, context.Canceled) {
				m.status = "Connection check canceled."
			}
			rejected := errors.Is(msg.err, gateway.ErrRejected) && (msg.kind != "save" || msg.usingSaved)
			if rejected || errors.Is(msg.err, credentials.ErrNotFound) {
				m.configured = false
				m.clearCatalog()
				m.apiKey.SetValue("")
			}
			return m, nil
		}
		switch msg.kind {
		case "browse":
			m.catalog = msg.catalog
			m.tree = navigation.Build(*msg.catalog)
			_, m.route = m.tree.Find(m.route)
			m.selected = 0
			m.details = false
			m.status = fmt.Sprintf("Catalog loaded: %d apps, %d tools. Navigation is local; R refreshes.", len(m.tree.Root.Children), msg.catalog.Report.ToolCount)
		case "remove":
			m.apiKey.SetValue("")
			m.status = "API key removed. Enter a new key to connect."
		default:
			m.configured = true
			if msg.kind == "save" {
				m.apiKey.SetValue("")
			}
			m.status = fmt.Sprintf("Connected to %s %s using MCP %s. %d tools discovered.", safeText(msg.report.ServerName), safeText(msg.report.ServerVersion), safeText(msg.report.Protocol), msg.report.ToolCount)
			if msg.kind == "save" && m.sessionOnly {
				m.status += " API key is available only for this session."
			}
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.origin.SetStyles(textinput.DefaultStyles(m.dark))
		m.apiKey.SetStyles(textinput.DefaultStyles(m.dark))
		m.search.SetStyles(textinput.DefaultStyles(m.dark))
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.origin.SetWidth(max(1, m.width-14))
		m.apiKey.SetWidth(max(1, m.width-14))
		m.search.SetWidth(max(1, m.width-14))
		m.resizeDetails()
		if m.activeRun != nil {
			m.formValue.SetWidth(max(1, m.contentWidth()-4))
			m.confirmValue.SetWidth(max(1, m.contentWidth()-15))
			m.formArea.SetWidth(m.contentWidth())
			m.formArea.SetHeight(max(1, min(5, m.menuHeight()-4)))
		}
		return m, nil
	case tea.PasteMsg:
		if m.activeRun != nil && !m.busy && m.activeRun.Mode == "input" {
			value := m.formValue.Value()
			f := m.activeRun.Current()
			if f != nil && (f.Control == "textarea" || f.Control == "json") {
				value = m.formArea.Value()
			}
			if utf8.RuneCountInString(value)+utf8.RuneCountInString(msg.Content) > 1<<20 {
				m.formError = "Input exceeds the 1,048,576-character limit. The paste was not applied."
				return m, nil
			}
		}
	case tea.KeyPressMsg:
		k := msg.String()
		if k == "ctrl+c" && m.busy && m.busyKind == "tool" {
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.status = "Canceling operation; its outcome may be unknown..."
			return m, nil
		}
		if k == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			m.apiKey.SetValue("")
			return m, tea.Quit
		}
		if m.busy {
			if k == "esc" && m.busyKind == "tool" && m.cancel != nil {
				m.cancel()
				m.cancel = nil
				m.status = "Canceling operation; its outcome may be unknown..."
				return m, nil
			}
			if k == "esc" && m.cancel != nil {
				if m.busyKind == "remove" {
					m.status = "Finishing credential removal..."
				} else {
					m.cancel()
					m.apiKey.SetValue("")
					m.status = "Canceling connection check..."
				}
			}
			return m, nil
		}
		if m.activeRun != nil {
			return m.interactionKey(msg)
		}
		if m.maintenancePage {
			return m.maintenanceKey(msg)
		}
		if m.settings {
			return m.settingsKey(msg)
		}
		if k == "f2" {
			cmd := m.openSettings()
			return m, cmd
		}
		if m.filtering {
			switch k {
			case "esc":
				m.filtering = false
				m.search.SetValue("")
				m.search.Blur()
				m.selected = 0
				return m, nil
			case "enter":
				m.filtering = false
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.selected = 0
			return m, cmd
		}

		if m.browser {
			return m.browserKey(msg)
		}
		switch k {
		case "up", "k":
			m.cursor = (m.cursor + 2) % 3
		case "down", "j":
			m.cursor = (m.cursor + 1) % 3
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = 2
		case "esc":
			return m, tea.Quit
		case "enter":
			switch m.cursor {
			case 0:
				if !m.configured {
					m.status = "Configure a Gateway and API key in Settings first."
					cmd := m.openSettings()
					return m, cmd
				}
				m.browser = true
				if m.catalog == nil {
					return m, m.operation("browse")
				}
			case 1:
				cmd := m.openSettings()
				return m, cmd
			case 2:
				return m, tea.Quit
			}
		}
		return m, nil
	}
	if m.activeRun != nil && !m.busy {
		var cmd tea.Cmd
		if m.activeRun.Mode == "input" {
			f := m.activeRun.Current()
			if f != nil && (f.Control == "textarea" || f.Control == "json") {
				m.formArea, cmd = m.formArea.Update(msg)
			} else {
				m.formValue, cmd = m.formValue.Update(msg)
			}
		} else if m.activeRun.Mode == "review" && m.activeRun.TypedConfirmation() {
			m.confirmValue, cmd = m.confirmValue.Update(msg)
		}
		return m, cmd
	}
	if m.filtering && !m.busy {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	if m.settings && !m.busy {
		var cmd tea.Cmd
		if m.focus == 0 {
			m.origin, cmd = m.origin.Update(msg)
		} else if m.focus == 1 {
			m.apiKey, cmd = m.apiKey.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m model) settingsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.confirmRemove {
		if k == "esc" {
			m.confirmRemove = false
		} else if k == "enter" {
			m.confirmRemove = false
			return m, m.operation("remove")
		}
		return m, nil
	}
	switch k {
	case "esc":
		m.closeSettings()
		return m, nil
	case "tab":
		m.focus = (m.focus + 1) % 8
		return m, m.focusInput()
	case "shift+tab":
		m.focus = (m.focus + 7) % 8
		return m, m.focusInput()
	case "enter":
		switch m.focus {
		case 0:
			m.focus = 1
			return m, m.focusInput()
		case 1, 3:
			return m, m.operation("save")
		case 2:
			m.sessionOnly = !m.sessionOnly
		case 4:
			return m, m.operation("test")
		case 5:
			m.confirmRemove = true
		case 7:
			m.closeSettings()
		case 6:
			m.maintenancePage = true
			m.maintenanceFocus = 0
			m.origin.Blur()
			m.apiKey.Blur()
		}
		return m, nil
	case "space":
		if m.focus == 2 {
			m.sessionOnly = !m.sessionOnly
			return m, nil
		}
	}
	var cmd tea.Cmd
	if m.focus == 0 {
		m.origin, cmd = m.origin.Update(msg)
	} else if m.focus == 1 {
		m.apiKey, cmd = m.apiKey.Update(msg)
	}
	return m, cmd
}

func (m model) currentNode() *navigation.Node {
	if m.tree.Root == nil {
		return nil
	}
	node, _ := m.tree.Find(m.route)
	return node
}
func (m model) menuRows() []*navigation.Node {
	node := m.currentNode()
	if node == nil {
		return nil
	}
	rows := []*navigation.Node{}
	if node.Action != nil && len(node.Children) > 0 {
		rows = append(rows, &navigation.Node{Label: "About this action", Action: node.Action})
	}
	rows = append(rows, node.Children...)
	query := strings.ToLower(strings.TrimSpace(safeText(m.search.Value())))
	if query == "" {
		return rows
	}
	filtered := []*navigation.Node{}
	for _, row := range rows {
		if strings.Contains(strings.ToLower(safeText(row.Label)+" "+safeText(row.ID)), query) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
func (m model) browserKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "esc" || k == "left" {
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.selected = 0
		} else if m.details {
			m.details = false
			if m.detailPushed && len(m.route) > 0 {
				m.backMenu()
			}
			m.detailPushed = false
		} else if len(m.route) > 0 {
			m.backMenu()
		} else {
			m.browser = false
			m.cursor = 0
		}
		return m, nil
	}
	if k == "?" && m.catalog != nil && !m.details {
		node := m.currentNode()
		lines := []string{"Menu information", "", "Only published CLI paths are shown. No tool has been executed."}
		issues := m.tree.Diagnostics
		if node != nil {
			issues = append(append([]string{}, issues...), node.Diagnostics...)
		}
		if len(issues) == 0 {
			lines = append(lines, "", "No menu metadata issues.")
		}
		for _, issue := range issues {
			lines = append(lines, "", safeText(issue))
		}
		m.details = true
		m.detailPushed = false
		m.detailAction = nil
		m.resizeDetails()
		m.detail.SetContent(strings.Join(lines, "\n"))
		m.detail.GotoTop()
		return m, nil
	}
	if k == "r" || k == "R" {
		if m.details && m.detailPushed && len(m.route) > 0 {
			m.backMenu()
		}
		m.detailPushed = false
		m.search.SetValue("")
		m.details = false
		return m, m.operation("browse")
	}
	if m.details && k == "enter" && m.detailAction != nil {
		return m, m.startInteraction()
	}
	if m.details {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	if k == "/" {
		m.filtering = true
		return m, m.search.Focus()
	}
	rows := m.menuRows()
	if len(rows) == 0 {
		return m, nil
	}
	switch k {
	case "up", "k":
		m.selected = max(0, m.selected-1)
	case "down", "j":
		m.selected = min(len(rows)-1, m.selected+1)
	case "home":
		m.selected = 0
	case "end":
		m.selected = len(rows) - 1
	case "pgup":
		m.selected = max(0, m.selected-m.menuPageSize())
	case "pgdown":
		m.selected = min(len(rows)-1, m.selected+m.menuPageSize())
	case "enter", "right":
		row := rows[min(m.selected, len(rows)-1)]
		m.search.SetValue("")
		m.selected = 0
		if row.ID != "" {
			m.route = append(append([]string{}, m.route...), row.ID)
		}
		if row.Action != nil && len(row.Children) == 0 {
			m.detailAction = row.Action
			m.detailPushed = row.ID != ""
			m.details = true
			m.resizeDetails()
			m.detail.SetContent(m.actionText(row))
			m.detail.GotoTop()
		}
	}
	return m, nil
}

func (m *model) backMenu() {
	if len(m.route) == 0 {
		return
	}
	id := m.route[len(m.route)-1]
	m.route = m.route[:len(m.route)-1]
	m.selected = 0
	for i, row := range m.menuRows() {
		if row.ID == id {
			m.selected = i
			break
		}
	}
}

func safeText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, ansi.Strip(value))
}

func Run(ctx context.Context, input io.Reader, output io.Writer, version string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	root, err := config.Root()
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(newModel(ctx, version, setup.New(root, version)), tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output)).Run()
	if result, ok := final.(model); ok && result.exitNotice != "" {
		fmt.Fprintln(output, result.exitNotice)
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return context.Canceled
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
