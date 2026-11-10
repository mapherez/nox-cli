package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/credentials"
	"github.com/mapherez/nox-cli/internal/gateway"
	"github.com/mapherez/nox-cli/internal/interaction"
	"github.com/mapherez/nox-cli/internal/navigation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type callProgress struct {
	generation int
	message    string
}
type callFinished struct {
	generation int
	result     *mcp.CallToolResult
	err        error
}

func (m *model) startInteraction() tea.Cmd {
	if m.detailAction == nil || m.catalog == nil {
		return nil
	}
	if m.detailAction.Problem != "" {
		m.setStatus("error", m.detailAction.Problem)
		return nil
	}
	run, err := interaction.New(*m.catalog, m.detailAction.Alias)
	if err != nil {
		m.setStatus("error", err.Error())
		return nil
	}
	m.activeRun = run
	m.formError = ""
	m.setStatus("", "")
	return m.syncInteraction()
}
func (m *model) syncInteraction() tea.Cmd {
	if m.activeRun == nil {
		return nil
	}
	r := m.activeRun
	m.formError = ""
	if r.DirectRead() {
		if err := r.Approve(""); err != nil {
			m.formError = err.Error()
			return nil
		}
	}
	switch r.Mode {
	case "pending":
		return m.startToolCall()
	case "input":
		m.formValue = textinput.New()
		m.formValue.Prompt = "> "
		m.formValue.CharLimit = 1 << 20
		m.formValue.SetWidth(max(1, m.contentWidth()-4))
		m.formValue.SetStyles(textinput.DefaultStyles(m.dark))
		m.formArea = textarea.New()
		m.formArea.ShowLineNumbers = false
		m.formArea.CharLimit = 1 << 20
		m.formArea.SetWidth(m.contentWidth())
		m.formArea.SetHeight(max(1, min(5, m.menuHeight()-4)))
		m.formArea.Prompt = "> "
		m.optionCursor = 0
		m.optionSelected = map[int]bool{}
		f := r.Current()
		if f == nil {
			return nil
		}
		if f.Secret {
			m.formValue.EchoMode = textinput.EchoPassword
		}
		if value, exists := interaction.Get(r.Arguments, f.Pointer); exists {
			raw, ok := value.(string)
			if !ok {
				raw = interaction.Pretty(value)
			}
			if f.Control != "select" && f.Control != "multiselect" && f.Control != "checkbox" {
				m.formValue.SetValue(raw)
				m.formArea.SetValue(raw)
			}
			for i, o := range m.currentOptions() {
				if interaction.Equal(o.Value, value) {
					m.optionCursor = i
				}
				for _, selected := range slice(value) {
					if interaction.Equal(o.Value, selected) {
						m.optionSelected[i] = true
					}
				}
			}
		}
		if f.Control == "textarea" || f.Control == "json" {
			return m.formArea.Focus()
		}
		return m.formValue.Focus()
	case "review":
		m.confirmValue = textinput.New()
		m.confirmValue.Prompt = "Confirmation: "
		m.confirmValue.CharLimit = 4096
		m.confirmValue.SetWidth(max(1, m.contentWidth()-15))
		m.confirmValue.SetStyles(textinput.DefaultStyles(m.dark))
		m.resizeDetails()
		m.detail.SetContent(m.reviewText())
		m.detail.GotoTop()
		if r.TypedConfirmation() {
			return m.confirmValue.Focus()
		}
	case "done", "error":
		m.resizeDetails()
		m.detail.SetContent(m.resultText())
		m.detail.GotoTop()
	}
	return nil
}
func slice(v any) []any { a, _ := v.([]any); return a }
func (m *model) startToolCall() tea.Cmd {
	r := m.activeRun
	if r == nil || r.Pending == nil {
		return nil
	}
	m.busy = true
	m.busyKind = "tool"
	m.generation++
	gen := m.generation
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	events := make(chan tea.Msg, 16)
	m.callEvents = events
	request := *r.Pending
	manager := m.manager
	root := m.ctx
	request.Progress = func(s string) {
		select {
		case events <- callProgress{gen, s}:
		default:
		}
	}
	go func() {
		defer cancel()
		result, err := manager.Call(ctx, request)
		select {
		case events <- callFinished{gen, result, err}:
		case <-root.Done():
		}
	}()
	m.setStatus("working", "Running your action through Gateway…")
	return m.awaitToolEvent()
}
func (m model) awaitToolEvent() tea.Cmd {
	events, ctx := m.callEvents, m.ctx
	return func() tea.Msg {
		select {
		case msg := <-events:
			return msg
		case <-ctx.Done():
			return nil
		}
	}
}
func (m model) interactionEvent(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case callProgress:
		if event.generation != m.generation || m.activeRun == nil {
			return m, nil
		}
		if m.cancel == nil {
			return m, m.awaitToolEvent()
		}
		m.setStatus("working", m.redact(safeText(event.message)))
		if m.status == "" {
			m.setStatus("working", "Running your action through Gateway…")
		}
		return m, m.awaitToolEvent()
	case callFinished:
		if event.generation != m.generation || m.activeRun == nil {
			return m, nil
		}
		m.busy = false
		m.cancel = nil
		m.callEvents = nil
		if m.activeRun.Pending != nil && !m.activeRun.Pending.ReadOnly {
			if m.catalog != nil {
				m.catalog.LoadedAt = time.Time{}
			}

		}
		m.activeRun.Receive(event.result, event.err)
		if errors.Is(event.err, gateway.ErrRejected) || errors.Is(event.err, credentials.ErrNotFound) {
			m.activeRun = nil
			m.formValue.SetValue("")
			m.formArea.SetValue("")
			m.confirmValue.SetValue("")
			m.optionSelected = nil
			m.clearCatalog()
			m.configured = false
			m.detailAction = nil
			m.setStatus("error", event.err.Error())
			m.lastReport = nil
			m.connectionIssue = "API key required or rejected"
			cmd := m.openSettings()
			return m, cmd
		}
		if errors.Is(event.err, gateway.ErrStale) || errors.Is(event.err, gateway.ErrAccess) {
			m.catalog = nil
			m.tree = navigation.Tree{}
			m.detailAction = nil
		}
		m.statusKind = ""
		m.status = ""
		if m.activeRun.Mode == "error" {
			m.setStatus("error", m.redact(m.activeRun.Problem))
		}
		cmd := m.syncInteraction()
		return m, cmd
	}
	return m, nil
}
func (m model) currentOptions() []interaction.Option {
	if m.activeRun == nil {
		return nil
	}
	r := m.activeRun
	f := r.Current()
	if f == nil {
		return nil
	}
	options := r.Options
	if f.Control == "checkbox" {
		options = []interaction.Option{{Label: "Enabled", Value: true}, {Label: "Disabled", Value: false}}
	}
	query := strings.ToLower(strings.TrimSpace(m.formValue.Value()))
	if query == "" || f.Control == "checkbox" {
		return options
	}
	filtered := []interaction.Option{}
	for _, o := range options {
		if strings.Contains(strings.ToLower(safeText(o.Label)+" "+interaction.Pretty(o.Value)), query) {
			filtered = append(filtered, o)
		}
	}
	return filtered
}
func (m model) interactionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := m.activeRun
	k := msg.String()
	if k == "esc" || (k == "enter" && (r.Mode == "done" || r.Mode == "error")) {
		m.setStatus("", "")
		m.activeRun = nil
		m.formValue.SetValue("")
		m.formArea.SetValue("")
		m.confirmValue.SetValue("")
		m.formError = ""
		if m.catalog == nil {
			m.details = false
			m.detailAction = nil
		} else if m.actionFromMenu {
			m.details = false
			m.detailAction = nil
			if m.detailPushed && len(m.route) > 0 {
				m.backMenu()
			}
			m.detailPushed = false
			m.actionFromMenu = false
		} else if m.detailAction != nil {
			label := m.detailAction.Alias
			if current := m.currentNode(); current != nil {
				label = current.Label
			}
			node := &navigation.Node{Label: label, Action: m.detailAction}
			m.detail.SetContent(m.actionText(node))
			m.detail.GotoTop()
		}
		return m, nil
	}
	if (r.Mode == "error" || r.Mode == "done") && (k == "r" || k == "R") {
		m.activeRun = nil
		m.actionFromMenu = false
		m.details = false
		m.detailAction = nil
		if m.detailPushed {
			m.backMenu()
			m.detailPushed = false
		}
		return m, m.operation("browse")
	}
	if r.Mode == "review" {
		switch k {
		case "ctrl+e":
			if err := r.Edit(); err != nil {
				m.formError = err.Error()
				return m, nil
			}
			return m, m.syncInteraction()
		case "enter":
			if err := r.Approve(m.confirmValue.Value()); err != nil {
				m.formError = err.Error()
				return m, nil
			}
			m.confirmValue.SetValue("")
			return m, m.syncInteraction()
		}
		if k == "up" || k == "down" || k == "pgup" || k == "pgdown" || k == "home" || k == "end" {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}
		if r.TypedConfirmation() {
			var cmd tea.Cmd
			m.confirmValue, cmd = m.confirmValue.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	if r.Mode == "done" || r.Mode == "error" {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	if r.Mode != "input" {
		return m, nil
	}
	f := r.Current()
	if f == nil {
		return m, nil
	}
	// Left returns to the previous field at the start of an editor. Elsewhere,
	// arrows retain their normal text-cursor behavior.
	if k == "left" {
		choices := f.Control == "select" || f.Control == "multiselect" || f.Control == "checkbox"
		atStart := m.formValue.Position() == 0
		if f.Control == "textarea" || f.Control == "json" {
			info := m.formArea.LineInfo()
			atStart = m.formArea.Line() == 0 && info.StartColumn == 0 && info.CharOffset == 0
		}
		if r.Index > 0 && (choices || atStart) {
			r.Back()
			return m, m.syncInteraction()
		}
	}
	if k == "ctrl+u" {
		if err := r.Set(nil, false); err != nil {
			m.formError = err.Error()
			return m, nil
		}
		return m, m.syncInteraction()
	}
	choices := f.Control == "select" || f.Control == "multiselect" || f.Control == "checkbox"
	if choices {
		options := m.currentOptions()
		switch k {
		case "up":
			m.optionCursor = max(0, m.optionCursor-1)
			return m, nil
		case "down":
			m.optionCursor = min(max(0, len(options)-1), m.optionCursor+1)
			return m, nil
		case "home":
			m.optionCursor = 0
			return m, nil
		case "end":
			m.optionCursor = max(0, len(options)-1)
			return m, nil
		case "pgup":
			m.optionCursor = max(0, m.optionCursor-m.menuPageSize())
			return m, nil
		case "pgdown":
			m.optionCursor = min(max(0, len(options)-1), m.optionCursor+m.menuPageSize())
			return m, nil
		case "space":
			if f.Control == "multiselect" && len(options) > 0 {
				target := options[min(m.optionCursor, len(options)-1)]
				for i, o := range r.Options {
					if interaction.Equal(o.Value, target.Value) {
						m.optionSelected[i] = !m.optionSelected[i]
					}
				}
				return m, nil
			}
		case "enter":
			if r.Required(*f) && len(r.Options) == 0 && (f.Control == "select" || f.Control == "multiselect") {
				m.formError = "No choices are available for this required field."
				return m, nil
			}
			var value any
			if f.Control == "multiselect" {
				selected := []any{}
				indices := []int{}
				for i, yes := range m.optionSelected {
					if yes {
						indices = append(indices, i)
					}
				}
				sort.Ints(indices)
				for _, i := range indices {
					selected = append(selected, r.Options[i].Value)
				}
				value = selected
			} else {
				if len(options) == 0 {
					m.formError = "No choices are available. Omit an optional field or go back."
					return m, nil
				}
				value = options[min(m.optionCursor, len(options)-1)].Value
			}
			if err := r.Set(value, true); err != nil {
				m.formError = err.Error()
				return m, nil
			}
			return m, m.syncInteraction()
		}
		if f.Control != "checkbox" {
			var cmd tea.Cmd
			m.formValue, cmd = m.formValue.Update(msg)
			m.optionCursor = 0
			return m, cmd
		}
		return m, nil
	}
	multiline := f.Control == "textarea" || f.Control == "json"
	if (!multiline && k == "enter") || (multiline && k == "ctrl+d") {
		raw := m.formValue.Value()
		if multiline {
			raw = m.formArea.Value()
		}
		value, err := f.Parse(raw)
		if err == nil {
			err = r.Set(value, true)
		}
		if err != nil {
			m.formError = err.Error()
			return m, nil
		}
		return m, m.syncInteraction()
	}
	var cmd tea.Cmd
	if multiline {
		m.formArea, cmd = m.formArea.Update(msg)
	} else {
		m.formValue, cmd = m.formValue.Update(msg)
	}
	return m, cmd
}
func (m model) redact(s string) string {
	if m.activeRun != nil {
		for _, secret := range m.activeRun.SecretValues() {
			if secret != "" {
				s = strings.ReplaceAll(s, secret, "[hidden]")
			}
		}
	}
	return s
}
func safeParagraph(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}
func (m model) reviewText() string {
	r := m.activeRun
	target := r.Catalog.Definitions[r.Target()]
	if target == nil {
		target = r.Catalog.Definitions[r.Alias]
	}
	label, description := r.Target(), ""
	if target != nil {
		if tool, ok := target["tool"].(map[string]any); ok {
			description, _ = tool["description"].(string)
		}
		if cli, ok := target["cli"].(map[string]any); ok {
			if nav, ok := cli["navigation"].(map[string]any); ok {
				if name, ok := nav["label"].(string); ok && name != "" {
					label = name
				}
			}
		}
	}
	lines := []string{"Review action", safeText(label), safeText(description), "", "Arguments", interaction.Pretty(r.ReviewArguments())}
	if r.Preview != nil {
		lines = append(lines, "", "Preview", interaction.Pretty(r.Preview))
	}
	if message := r.ConfirmationMessage(); message != "" {
		lines = append(lines, "", message)
	}
	lines = append(lines, "", "This action requires your confirmation.")
	return m.redact(safeParagraph(strings.Join(lines, "\n")))
}
func (m model) resultText() string {
	r := m.activeRun
	label := r.Alias
	if node := m.currentNode(); node != nil && node.Action != nil && node.Action.Alias == r.Alias {
		label = node.Label
	}
	lines := []string{safeText(label), ""}
	if r.Problem != "" {
		lines = append(lines, r.Problem, "")
	}
	if r.Result != nil {
		schema := any(nil)
		name := r.Target()
		if name == "" {
			name = r.Alias
		}
		if definition := r.Catalog.Definitions[name]; definition != nil {
			if tool, ok := definition["tool"].(map[string]any); ok {
				schema = tool["outputSchema"]
			}
		}
		if r.Result.StructuredContent != nil {
			lines = append(lines, readableResult(r.Result.StructuredContent, schema), "")
		}
		for _, content := range r.Result.Content {
			switch c := content.(type) {
			case *mcp.TextContent:
				if value, err := interaction.Decode(c.Text); err == nil {
					if r.Result.StructuredContent != nil && interaction.Equal(value, r.Result.StructuredContent) {
						continue
					}
					lines = append(lines, readableResult(value, nil))
				} else {
					lines = append(lines, c.Text)
				}
			case *mcp.ImageContent:
				lines = append(lines, fmt.Sprintf("Image: %s (%d bytes)", c.MIMEType, len(c.Data)))
			case *mcp.AudioContent:
				lines = append(lines, fmt.Sprintf("Audio: %s (%d bytes)", c.MIMEType, len(c.Data)))
			case *mcp.ResourceLink:
				lines = append(lines, "Resource: "+c.Name+" · "+c.URI)
			case *mcp.EmbeddedResource:
				if c.Resource != nil {
					lines = append(lines, "Embedded resource: "+c.Resource.URI+" · "+c.Resource.MIMEType)
					if c.Resource.Text != "" {
						lines = append(lines, c.Resource.Text)
					}
					if len(c.Resource.Blob) > 0 {
						lines = append(lines, fmt.Sprintf("Binary resource (%d bytes)", len(c.Resource.Blob)))
					}
				}
			default:
				lines = append(lines, "Additional MCP content is not rendered in this terminal.")
			}
		}
	}
	return m.redact(safeParagraph(strings.TrimSpace(strings.Join(lines, "\n"))))
}

func (m model) interactionLines() ([]string, int) {
	r := m.activeRun
	switch r.Mode {
	case "pending":
		return []string{"An operation is in progress.", "", "A canceled write may already have completed."}, 0
	case "review":
		lines := strings.Split(m.detail.View(), "\n")
		if r.TypedConfirmation() {
			lines = append([]string{m.confirmValue.View(), ""}, lines...)
		}

		return lines, 0
	case "done", "error":
		return strings.Split(m.detail.View(), "\n"), 0
	case "input":
		f := r.Current()
		if f == nil {
			return []string{"No input available."}, 0
		}
		kind := "optional"
		if r.Required(*f) {
			kind = "required"
		}
		lines := []string{m.palette().secondary.Bold(true).Render(fmt.Sprintf("%s · %s · %d/%d", safeText(f.Label), kind, r.Index+1, len(r.Fields))), safeText(f.Description), ""}
		focus := len(lines)
		if f.Control == "select" || f.Control == "multiselect" || f.Control == "checkbox" {
			if f.Control != "checkbox" {
				lines = append(lines, m.formValue.View(), "")
				focus = len(lines)
			}
			options := m.currentOptions()
			if len(options) == 0 {
				lines = append(lines, "No choices available.")
			}
			for i, o := range options {
				label := safeText(o.Label)
				if f.Control == "multiselect" {
					marker := "[ ] "
					for j, original := range r.Options {
						if m.optionSelected[j] && interaction.Equal(original.Value, o.Value) {
							marker = "[x] "
						}
					}
					label = marker + label
				}
				lines = append(lines, m.menuChoice(label, i == m.optionCursor))
			}
			focus += m.optionCursor
		} else if f.Control == "textarea" || f.Control == "json" {
			lines = append(lines, strings.Split(m.formArea.View(), "\n")...)
		} else {
			lines = append(lines, m.formValue.View())
		}

		return lines, focus
	}
	return []string{"Unsupported interaction."}, 0
}
