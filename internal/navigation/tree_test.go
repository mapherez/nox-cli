package navigation

import (
	"encoding/json"
	"testing"

	"github.com/mapherez/nox-cli/internal/gateway"
	noxmcp "github.com/mapherez/nox-mcp/go"
)

func menuCatalog(t *testing.T) gateway.Catalog {
	t.Helper()
	// A small contract fixture using real Backend names, not shipped product data.
	raw := `{"presentation":{"products":[{"productId":"nox-backend","displayName":"NoX Backend"}]},"definitions":{
 "alias-list":{"tool":{"name":"alias-list","_meta":{"cli":"vault list","nox/gateway":{"version":"1.0","productId":"nox-backend"}}},"cli":{"version":"1.0","navigation":{"label":"List","group":"Vaults","order":40}}},
 "alias-create":{"tool":{"name":"alias-create","_meta":{"cli":"vault create","nox/gateway":{"version":"1.0","productId":"nox-backend"}}},"cli":{"version":"1.0","navigation":{"label":"Create","group":"Vaults","order":50}}},
 "alias-status":{"tool":{"name":"alias-status","_meta":{"cli":"health","nox/gateway":{"version":"1.0","productId":"nox-backend"}}},"cli":{"version":"1.0","navigation":{"label":"Service status","order":10}}}
 }}`
	var input struct {
		Presentation noxmcp.JSON
		Definitions  map[string]noxmcp.JSON
	}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	return gateway.Catalog{Presentation: input.Presentation, Definitions: input.Definitions}
}

func TestCanonicalPathsLabelsOrderAndIdentity(t *testing.T) {
	c := menuCatalog(t)
	tree := Build(c)
	app := tree.Root.Children[0]
	if app.ID != "nox-backend" || app.Label != "NoX Backend" || len(app.Children) != 2 {
		t.Fatalf("app: %+v", app)
	}
	if app.Children[0].Label != "Service status" || app.Children[1].ID != "vault" || app.Children[1].Label != "Vaults" {
		t.Fatal("labels or descriptor order lost")
	}
	vaults := app.Children[1]
	if len(vaults.Children) != 2 || vaults.Children[0].Label != "List" || vaults.Children[1].Label != "Create" {
		t.Fatal("duplicate group layer or unstable order")
	}
	if vaults.Children[0].Action.Alias != "alias-list" {
		t.Fatal("canonical gateway alias lost")
	}
	node, route := tree.Find([]string{"nox-backend", "vault", "removed"})
	if node != vaults || len(route) != 2 {
		t.Fatal("refresh did not find the surviving ancestor")
	}
	if _, ok := c.Definitions["alias-list"]["cli"].(map[string]any)["parameters"]; ok {
		t.Fatal("source descriptor mutated")
	}
}

func TestIndividualMetadataFailuresDoNotHideOtherActions(t *testing.T) {
	c := menuCatalog(t)
	c.Definitions["alias-create"]["cli"].(map[string]any)["requires"] = []any{"future-interaction"}
	delete(c.Definitions["alias-status"]["tool"].(map[string]any)["_meta"].(map[string]any), "cli")
	tree := Build(c)
	app := tree.Root.Children[0]
	if len(app.Diagnostics) != 1 {
		t.Fatal("missing path not reported")
	}
	vaults := app.Children[0]
	if len(vaults.Children) != 2 {
		t.Fatal("one bad descriptor hid valid tools")
	}
	var allowed, blocked bool
	for _, n := range vaults.Children {
		if n.Action.Alias == "alias-list" {
			allowed = n.Action.Problem == ""
		}
		if n.Action.Alias == "alias-create" {
			blocked = n.Action.Problem != ""
		}
	}
	if !allowed || !blocked {
		t.Fatal("unsupported features not isolated")
	}
}

func TestActionWithChildrenCollisionAndMissingOrigin(t *testing.T) {
	c := menuCatalog(t)
	c.Definitions["alias-status"]["tool"].(map[string]any)["_meta"].(map[string]any)["cli"] = "vault"
	tree := Build(c)
	node, _ := tree.Find([]string{"nox-backend", "vault"})
	if node.Action == nil || len(node.Children) != 2 {
		t.Fatal("action-with-subcommands lost")
	}
	c.Definitions["duplicate"] = c.Definitions["alias-list"]
	tree = Build(c)
	node, _ = tree.Find([]string{"nox-backend", "vault", "list"})
	if node.Action.Problem == "" || len(node.Diagnostics) == 0 {
		t.Fatal("ambiguous path silently selected a tool")
	}
	delete(c.Definitions["alias-create"]["tool"].(map[string]any)["_meta"].(map[string]any), "nox/gateway")
	tree = Build(c)
	if len(tree.Diagnostics) != 1 {
		t.Fatal("missing identity silently inferred from alias")
	}
}
