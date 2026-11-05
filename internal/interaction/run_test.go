package interaction

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mapherez/nox-cli/internal/gateway"
	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Focused descriptor fixtures; no product/runtime contracts are generated.
func definition(name string, schema, cli map[string]any, read bool) noxmcp.JSON {
	d := noxmcp.JSON{"fingerprint": strings.Repeat("a", 43), "tool": map[string]any{"name": name, "inputSchema": schema, "annotations": map[string]any{"readOnlyHint": read}, "_meta": map[string]any{"cli": name, "nox/gateway": map[string]any{"version": "1.0", "productId": "contract-test", "serverId": "one-server"}}}}
	if cli != nil {
		d["cli"] = cli
	}
	return d
}
func schema(properties map[string]any, required ...string) map[string]any {
	names := []any{}
	for _, s := range required {
		names = append(names, s)
	}
	return map[string]any{"type": "object", "properties": properties, "required": names, "additionalProperties": false}
}
func catalog(defs ...noxmcp.JSON) gateway.Catalog {
	c := gateway.Catalog{ServerID: "test", ViewID: "account", LoadedAt: time.Now(), TTL: time.Minute, Definitions: map[string]noxmcp.JSON{}}
	for _, d := range defs {
		c.Definitions[text(object(d["tool"])["name"])] = d
	}
	return c
}
func mustRun(t *testing.T, c gateway.Catalog, name string) *Run {
	t.Helper()
	r, e := New(c, name)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func mustSet(t *testing.T, r *Run, v any, present bool) {
	t.Helper()
	if e := r.Set(v, present); e != nil {
		t.Fatal(e)
	}
}
func result(v any) *mcp.CallToolResult { return &mcp.CallToolResult{StructuredContent: v} }

func selectorFixture(required bool) gateway.Catalog {
	requiredFields := []string{"project"}
	if required {
		requiredFields = append(requiredFields, "item")
	}
	cli := map[string]any{"version": "1.0", "parameters": map[string]any{"/project": map[string]any{"order": float64(100)}, "/item": map[string]any{"order": float64(1), "options": map[string]any{"tool": "options", "input": []any{"/project"}}}}}
	return catalog(definition("action", schema(map[string]any{"project": map[string]any{"type": "string"}, "item": map[string]any{"type": "string"}}, requiredFields...), cli, false), definition("options", schema(map[string]any{"project": map[string]any{"type": "string"}}, "project"), nil, true))
}
func TestSelectorDependenciesIdentityEmptyAndEditInvalidation(t *testing.T) {
	r := mustRun(t, selectorFixture(true), "action")
	if r.Current().Pointer != "/project" {
		t.Fatal("dependency not ordered before selector")
	}
	mustSet(t, r, "first", true)
	if r.Mode != "pending" || r.Pending.Name != "options" || r.Pending.Arguments["project"] != "first" {
		t.Fatal("wrong selector binding")
	}
	r.Receive(result(map[string]any{"items": []any{map[string]any{"id": "one", "name": "Same"}, map[string]any{"id": "two", "name": "Same"}}}), nil)
	if r.Options[0].Label == r.Options[1].Label {
		t.Fatal("duplicate labels not distinguished")
	}
	if r.Set("unlisted", true) == nil {
		t.Fatal("unpublished selector value accepted")
	}
	mustSet(t, r, "two", true)
	if r.Mode != "review" || r.Arguments["item"] != "two" {
		t.Fatal("label used instead of ID")
	}
	if e := r.Edit(); e != nil {
		t.Fatal(e)
	}
	mustSet(t, r, "changed", true)
	if _, exists := r.Arguments["item"]; exists || r.Mode != "pending" || r.Pending.Arguments["project"] != "changed" {
		t.Fatal("dependent value or approval survived an edit")
	}
	r.Receive(result(map[string]any{}), nil)
	if r.Mode != "input" || len(r.Options) != 0 || r.Set("one", true) == nil || r.Set(nil, false) == nil {
		t.Fatal("empty required selector allowed execution")
	}
	optional := mustRun(t, selectorFixture(false), "action")
	mustSet(t, optional, "p", true)
	optional.Receive(result(map[string]any{}), nil)
	mustSet(t, optional, nil, false)
	if optional.Mode != "review" {
		t.Fatal("empty optional selector could not be omitted")
	}
}
func TestPreviewTypedConfirmationSecretsAndReviewGuard(t *testing.T) {
	cli := map[string]any{"version": "1.0", "parameters": map[string]any{"/password": map[string]any{"control": "password", "order": float64(20)}}, "preview": map[string]any{"tool": "preview", "input": []any{"/name"}, "bind": map[string]any{"/revision": "/revision"}, "reviewPointer": "/summary"}, "confirmation": map[string]any{"mode": "typed", "matchPointer": "/name", "bindPointer": "/confirm", "message": "Confirm this item."}}
	c := catalog(definition("action", schema(map[string]any{"name": map[string]any{"type": "string"}, "password": map[string]any{"type": "string"}, "revision": map[string]any{"type": "string"}, "confirm": map[string]any{"type": "boolean", "const": true}}, "name", "password", "revision", "confirm"), cli, false), definition("preview", schema(map[string]any{"name": map[string]any{"type": "string"}}, "name"), nil, true))
	r := mustRun(t, c, "action")
	mustSet(t, r, "item", true)
	if !r.Current().Secret {
		t.Fatal("descriptor password control was not masked")
	}
	mustSet(t, r, "secret-value", true)
	r.Receive(result(map[string]any{"revision": "v2", "summary": map[string]any{"name": "item"}}), nil)
	if r.Mode != "review" || r.Arguments["revision"] != "v2" || strings.Contains(Pretty(r.ReviewArguments()), "secret-value") {
		t.Fatal("preview binding or redaction failed")
	}
	if r.Approve("wrong") == nil || r.Pending != nil {
		t.Fatal("wrong typed approval executed")
	}
	r.Arguments["name"] = "changed"
	if r.Approve("changed") == nil {
		t.Fatal("changed arguments reused confirmation")
	}
	r.Arguments["name"] = "item"
	if e := r.Approve("item"); e != nil {
		t.Fatal(e)
	}
	if r.Pending == nil || r.Pending.Arguments["confirm"] != true || r.Pending.Arguments["password"] != "secret-value" || !r.Pending.Revalidate {
		t.Fatal("exact reviewed arguments not dispatched")
	}
}
func TestSequentialWorkflowBindingsFailureAndNoMutationReplay(t *testing.T) {
	steps := []any{map[string]any{"id": "input", "type": "input", "parameters": []any{"/query"}}, map[string]any{"id": "lookup", "type": "call", "tool": "lookup", "input": []any{"/query"}}, map[string]any{"id": "confirm", "type": "confirm", "confirmation": map[string]any{"mode": "boolean", "bindPointer": "/confirm"}}, map[string]any{"id": "write", "type": "call", "tool": "write", "arguments": map[string]any{"id": map[string]any{"from": "step", "step": "lookup", "pointer": "/id"}, "confirm": map[string]any{"from": "input", "pointer": "/confirm"}}}}
	c := catalog(definition("action", schema(map[string]any{"query": map[string]any{"type": "string"}, "confirm": map[string]any{"type": "boolean"}}, "query"), map[string]any{"version": "1.0", "workflow": steps}, false), definition("lookup", schema(map[string]any{"query": map[string]any{"type": "string"}}, "query"), nil, true), definition("write", schema(map[string]any{"id": map[string]any{"type": "integer"}, "confirm": map[string]any{"type": "boolean", "const": true}}, "id", "confirm"), nil, false))
	r := mustRun(t, c, "action")
	mustSet(t, r, "q", true)
	if r.Pending == nil || r.Pending.Name != "lookup" {
		t.Fatal("workflow lookup missing")
	}
	r.Receive(result(map[string]any{"id": json.Number("9007199254740993")}), nil)
	if r.Mode != "review" {
		t.Fatal("workflow confirmation missing")
	}
	if e := r.Approve(""); e != nil {
		t.Fatal(e)
	}
	if r.Mode != "review" || r.callName != "write" {
		t.Fatal("write was executed before review")
	}
	if e := r.Edit(); e != nil {
		t.Fatal(e)
	}
	if r.Mode != "input" || r.step != 0 {
		t.Fatal("editing skipped a workflow call")
	}
	mustSet(t, r, "q", true)
	r.Receive(result(map[string]any{"id": json.Number("9007199254740993")}), nil)
	_ = r.Approve("")
	_ = r.Approve("")
	if r.Pending == nil || r.Pending.Arguments["id"] != json.Number("9007199254740993") || r.Pending.Arguments["confirm"] != true {
		t.Fatal("workflow bindings lost types or confirmation")
	}
	r.Receive(&mcp.CallToolResult{IsError: true, StructuredContent: map[string]any{"code": "CONFLICT"}}, nil)
	if r.Mode != "error" || r.Pending != nil || r.Edit() == nil {
		t.Fatal("failed write replayed or workflow continued")
	}
}
func TestDefaultsTypesPointersAndUnsupportedReferences(t *testing.T) {
	raw := map[string]any{"a/b": map[string]any{"type": "integer", "default": 42}, "optional": map[string]any{"type": []any{"string", "null"}}}
	r := mustRun(t, catalog(definition("action", schema(raw, "a/b"), nil, true)), "action")
	if len(r.Arguments) != 0 || r.Current().Pointer != "/a~1b" {
		t.Fatal("default inserted or pointer escaped incorrectly")
	}
	v, e := r.Current().Parse("9007199254740993")
	if e != nil {
		t.Fatal(e)
	}
	mustSet(t, r, v, true)
	mustSet(t, r, nil, true)
	if r.Arguments["a/b"] != json.Number("9007199254740993") || r.Arguments["optional"] != nil {
		t.Fatal("JSON types changed")
	}
	if _, ok := Get(map[string]any{"a/b": []any{json.Number("123")}}, "/a~1b/0"); !ok {
		t.Fatal("pointer read failed")
	}
	c := selectorFixture(true)
	other := c.Definitions["options"]
	object(object(other["tool"])["_meta"])["nox/gateway"].(map[string]any)["productId"] = "another-product"
	if _, e := New(c, "action"); e == nil {
		t.Fatal("cross-product reference allowed")
	}
	c = selectorFixture(true)
	object(c.Definitions["action"]["cli"])["requires"] = []any{"unknown-feature"}
	if _, e := New(c, "action"); e == nil {
		t.Fatal("unknown required feature allowed")
	}
}
func TestDuplicateIDsCyclesOutputValidationAndFailedRead(t *testing.T) {
	source := map[string]any{"itemsPointer": "/items", "valuePointer": "/id", "labelPointer": "/name"}
	if _, e := parseOptions(source, map[string]any{"items": []any{map[string]any{"id": "x"}, map[string]any{"id": "x"}}}); e == nil {
		t.Fatal("duplicate IDs accepted")
	}
	c := selectorFixture(true)
	parameters := object(object(c.Definitions["action"]["cli"])["parameters"])
	parameters["/project"] = map[string]any{"options": map[string]any{"tool": "options", "input": []any{"/item"}}}
	if _, e := New(c, "action"); e == nil {
		t.Fatal("dependency cycle accepted")
	}
	c = selectorFixture(true)
	r := mustRun(t, c, "action")
	mustSet(t, r, "project", true)
	r.Receive(&mcp.CallToolResult{IsError: true}, nil)
	if r.Mode != "error" || r.Pending != nil {
		t.Fatal("failed selector continued")
	}
	d := definition("read", schema(map[string]any{}), nil, true)
	object(d["tool"])["outputSchema"] = map[string]any{"type": "object", "required": []any{"value"}}
	r = mustRun(t, catalog(d), "read")
	if e := r.Approve(""); e != nil {
		t.Fatal(e)
	}
	r.Receive(result(map[string]any{}), nil)
	if r.Mode != "error" {
		t.Fatal("invalid output schema accepted")
	}
}

func TestArrayPointersAndRecursiveFormsBlockCleanly(t *testing.T) {
	args := map[string]any{"items": []any{map[string]any{"id": "one"}}}
	if e := Put(args, "/items/0/token", "revision", true); e != nil {
		t.Fatal(e)
	}
	if v, ok := Get(args, "/items/0/token"); !ok || v != "revision" {
		t.Fatal("array binding failed")
	}
	if e := Put(args, "/items/0/token", nil, false); e != nil {
		t.Fatal(e)
	}
	recursive := schema(map[string]any{"child": map[string]any{"$ref": "#"}})
	if _, e := New(catalog(definition("recursive", recursive, nil, true)), "recursive"); e == nil {
		t.Fatal("recursive form should not loop indefinitely")
	}
	if Validate(map[string]any{"type": "number"}, json.Number("1e999999999")) == nil {
		t.Fatal("unbounded numeric exponent accepted")
	}
}

func TestRootConfirmationBindingIsNotSilentlyIgnored(t *testing.T) {
	cli := map[string]any{"version": "1.0", "confirmation": map[string]any{"mode": "boolean", "bindPointer": ""}}
	r := mustRun(t, catalog(definition("action", schema(map[string]any{}), cli, false)), "action")
	if r.Approve("") == nil || r.Pending != nil {
		t.Fatal("invalid root boolean binding was silently skipped")
	}
}
