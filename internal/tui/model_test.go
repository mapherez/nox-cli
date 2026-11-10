package tui

import (
	"charm.land/lipgloss/v2"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/mapherez/nox-cli/internal/config"
	"github.com/mapherez/nox-cli/internal/credentials"
	"github.com/mapherez/nox-cli/internal/gateway"
	noxmcp "github.com/mapherez/nox-mcp/go"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mapherez/nox-cli/internal/setup"
)

func TestEnteredAPIKeyNeverAppearsInTerminalView(t *testing.T) {
	manager := setup.New(t.TempDir(), "test")
	m := newModel(context.Background(), "test", manager)
	m.settings = true
	secret := "noxsync_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	m.apiKey.SetValue(secret)
	view := m.View().Content
	if strings.Contains(view, secret) || strings.Contains(view, secret[8:]) {
		t.Fatal("masked API key exposed in terminal rendering")
	}
	if safeText("a\x1b]52;c;dGVzdA==\x07\x1b[31mb\r\nc") != "abc" {
		t.Fatal("untrusted terminal controls were not removed")
	}
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if cmd != nil || updated.(model).apiKey.Value() != "" {
		t.Fatal("leaving Settings retained the typed key")
	}
}

type testVault struct{ secret credentials.Secret }

func (v *testVault) Get() (credentials.Secret, error) { return v.secret, nil }
func (v *testVault) Set(s credentials.Secret) error   { v.secret = s; return nil }
func (v *testVault) Delete() error                    { v.secret = credentials.Secret{}; return nil }

func browserFixture(t *testing.T, count int) (model, *int) {
	t.Helper()
	manager := setup.New(t.TempDir(), "test")
	manager.Vault = &testVault{secret: credentials.Secret{Origin: "http://localhost:5711", Key: "test-key"}}
	if err := manager.Store.Save(config.Settings{Version: 1, GatewayOrigin: "http://localhost:5711", CredentialRef: config.CredentialRef}); err != nil {
		t.Fatal(err)
	}
	calls := new(int)
	manager.Discover = func(context.Context, string, string, string) (gateway.Catalog, error) {
		*calls++
		definitions := map[string]noxmcp.JSON{}
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("alias-%03d", i)
			definitions[name] = noxmcp.JSON{"tool": map[string]any{"name": name, "description": strings.Repeat("Readable tool description. ", 30), "inputSchema": map[string]any{"type": "object"}, "_meta": map[string]any{"cli": fmt.Sprintf("vault action%d", i), "nox/gateway": map[string]any{"version": "1.0", "productId": "nox-backend"}}}, "cli": map[string]any{"version": "1.0", "navigation": map[string]any{"label": fmt.Sprintf("Action %03d", i), "group": "Vaults", "order": float64(i)}}}
		}
		return gateway.Catalog{Definitions: definitions, Report: gateway.Report{ToolCount: count}, Presentation: noxmcp.JSON{"products": []any{map[string]any{"productId": "nox-backend", "displayName": "NoX Backend"}}}}, nil
	}
	manager.Probe = func(context.Context, string, string, string) (gateway.Report, error) {
		t.Fatal("menu navigation performed a connection test")
		return gateway.Report{}, nil
	}
	return newModel(context.Background(), "test", manager), calls
}

func press(m model, k string) (model, tea.Cmd) {
	key := tea.Key{}
	switch k {
	case "enter":
		key.Code = tea.KeyEnter
	case "esc":
		key.Code = tea.KeyEsc
	case "down":
		key.Code = tea.KeyDown
	case "up":
		key.Code = tea.KeyUp
	case "left":
		key.Code = tea.KeyLeft
	case "right":
		key.Code = tea.KeyRight
	case "end":
		key.Code = tea.KeyEnd
	case "pgdown":
		key.Code = tea.KeyPgDown
	case "ctrl+d":
		key.Code = 'd'
		key.Mod = tea.ModCtrl
	case "ctrl+u":
		key.Code = 'u'
		key.Mod = tea.ModCtrl
	case "ctrl+e":
		key.Code = 'e'
		key.Mod = tea.ModCtrl
	case "shift+tab":
		key.Code = tea.KeyTab
		key.Mod = tea.ModShift
	case "space":
		key.Code = ' '
		key.Text = " "
	case "f2":
		key.Code = tea.KeyF2
	default:
		key.Code = []rune(k)[0]
		key.Text = k
	}
	updated, cmd := m.Update(tea.KeyPressMsg(key))
	return updated.(model), cmd
}
func loadApps(t *testing.T, m model) model {
	t.Helper()
	m, cmd := press(m, "enter")
	if cmd == nil || !m.busy {
		t.Fatal("Apps did not load asynchronously")
	}
	updated, _ := m.Update(cmd())
	return updated.(model)
}

func TestLocalMenuNavigationSearchDetailsBackAndExplicitRefresh(t *testing.T) {
	m, calls := browserFixture(t, 3)
	m = loadApps(t, m)
	for i := 0; i < 2; i++ {
		var cmd tea.Cmd
		m, cmd = press(m, "enter")
		if cmd != nil {
			t.Fatal("cached navigation made a request")
		}
	}
	if m.breadcrumb() != "Home / Apps / NoX Backend / Vaults" || !strings.Contains(m.View().Content, "Home / Apps / NoX Backend / Vaults") {
		t.Fatal(m.breadcrumb())
	}
	m, _ = press(m, "/")
	for _, r := range "002" {
		m, _ = press(m, string(r))
	}
	if len(m.menuRows()) != 1 {
		t.Fatal("local filter failed", m.search.Value())
	}
	m, _ = press(m, "enter")
	m, _ = press(m, "enter")
	if !m.details || !strings.Contains(m.detail.GetContent(), "alias-002") || *calls != 1 {
		t.Fatal("tool details or local navigation failed")
	}
	m, _ = press(m, "esc")
	if m.details || m.breadcrumb() != "Home / Apps / NoX Backend / Vaults" || m.selected != 2 {
		t.Fatal("action back did not restore the parent selection", m.breadcrumb(), m.selected)
	}
	m, _ = press(m, "esc")
	m, _ = press(m, "esc")
	m, _ = press(m, "esc")
	if m.browser {
		t.Fatal("back did not return Home")
	}
	m, _ = press(m, "enter")
	if *calls != 1 {
		t.Fatal("returning to Apps re-fetched the catalog")
	}
	m, cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("refresh did not request a new snapshot")
	}
	updated, _ := m.Update(cmd())
	m = updated.(model)
	if *calls != 2 || m.catalog == nil {
		t.Fatal("explicit refresh failed")
	}
}

func TestResizeLongLabelsScrollingAndTerminalControlSafety(t *testing.T) {
	m, _ := browserFixture(t, 80)
	m = loadApps(t, m)
	m, _ = press(m, "enter")
	m, _ = press(m, "enter")
	m, _ = press(m, "end")
	for _, size := range [][2]int{{90, 28}, {26, 12}, {9, 5}, {1, 1}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(model)
		content := m.View().Content
		if lipgloss.Height(content) > size[1] {
			t.Fatal("view exceeds terminal height", size, lipgloss.Height(content))
		}
		for _, line := range strings.Split(content, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("view exceeds terminal width", size, line)
			}
		}
		if size[0] > 20 && !strings.Contains(content, "Action 079") {
			t.Fatal("scrolling lost selected row", size, content)
		}
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = updated.(model)
	m.currentNode().Children[m.selected].Label = "\x1b]52;c;dGVzdA==\a" + strings.Repeat("Long ", 80)
	if strings.Contains(m.View().Content, "52;c;") {
		t.Fatal("untrusted label emitted terminal controls")
	}
	m, _ = press(m, "enter")
	m, _ = press(m, "pgdown")
	if !m.details || m.detail.YOffset() == 0 {
		t.Fatal("long detail description did not scroll")
	}
	m, _ = press(m, "r")
	if m.details || len(m.route) != 2 {
		t.Fatal("refresh from a leaf retained an empty leaf menu")
	}
}

func TestRejectedRefreshClearsMenusAndSettingsStayAccessible(t *testing.T) {
	m, calls := browserFixture(t, 1)
	m = loadApps(t, m)
	m.manager.Discover = func(context.Context, string, string, string) (gateway.Catalog, error) {
		*calls++
		return gateway.Catalog{}, gateway.ErrRejected
	}
	m, cmd := press(m, "r")
	updated, _ := m.Update(cmd())
	m = updated.(model)
	if m.catalog != nil || m.configured {
		t.Fatal("rejected credentials retained usable menu state")
	}
	m, cmd = press(m, "r")
	updated, _ = m.Update(cmd())
	m = updated.(model)
	if *calls != 2 {
		t.Fatal("rejected key was retried")
	}
	m, cmd = press(m, "f2")
	if cmd != nil {
		_ = cmd()
	} // input focus only
	if !m.settings {
		t.Fatal("Settings inaccessible after rejection")
	}
}

func TestUnconfiguredEmptyAndOfflineStates(t *testing.T) {
	m := newModel(context.Background(), "test", setup.New(t.TempDir(), "test"))
	m.cursor = 0
	m, _ = press(m, "enter")
	if !m.settings || m.busy {
		t.Fatal("unconfigured Apps attempted discovery")
	}
	m, _ = browserFixture(t, 0)
	m = loadApps(t, m)
	if !strings.Contains(m.View().Content, "NoX Backend") {
		t.Fatal("empty registered product not visible")
	}
	m, _ = press(m, "enter")
	if !strings.Contains(m.View().Content, "no navigable tools") {
		t.Fatal("empty product state missing")
	}
	m.manager.Discover = func(context.Context, string, string, string) (gateway.Catalog, error) {
		return gateway.Catalog{}, gateway.ErrUnavailable
	}
	m, cmd := press(m, "r")
	updated, _ := m.Update(cmd())
	m = updated.(model)
	if m.catalog != nil || !m.configured || !strings.Contains(m.View().Content, "No catalog loaded") {
		t.Fatal("outage confused with rejection")
	}
}

func TestSettingsInterruptsFilterWithoutTrappingHomeInput(t *testing.T) {
	m, _ := browserFixture(t, 1)
	m = loadApps(t, m)
	m, _ = press(m, "/")
	m, _ = press(m, "f2")
	if !m.settings || m.filtering {
		t.Fatal("Settings retained active search input")
	}
	m.clearCatalog()
	m.closeSettings()
	m, _ = press(m, "down")
	if m.browser || m.cursor != 1 || m.filtering {
		t.Fatal("returning Home trapped input in hidden search")
	}
}

func TestCanceledAndStaleCatalogCannotReplaceCurrentMenu(t *testing.T) {
	m, _ := browserFixture(t, 1)
	m.manager.Discover = func(ctx context.Context, _, _, _ string) (gateway.Catalog, error) {
		<-ctx.Done()
		return gateway.Catalog{}, ctx.Err()
	}
	m, cmd := press(m, "enter")
	m, _ = press(m, "esc")
	updated, _ := m.Update(cmd())
	m = updated.(model)
	if m.busy || m.catalog != nil || !strings.Contains(m.status, "canceled") {
		t.Fatal("canceled load published menus")
	}
	updated, _ = m.Update(checkedMsg{generation: m.generation - 1, kind: "browse", catalog: &gateway.Catalog{}})
	if updated.(model).catalog != nil {
		t.Fatal("stale result replaced menu state")
	}
}
