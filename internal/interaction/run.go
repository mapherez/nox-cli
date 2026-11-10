package interaction

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mapherez/nox-cli/internal/gateway"
	noxmcp "github.com/mapherez/nox-mcp/go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Run struct {
	secrets                []string
	Catalog                gateway.Catalog
	Alias, Mode, Problem   string
	Arguments              map[string]any
	Fields                 []Field
	Index                  int
	Options                []Option
	Preview                any
	Result                 *mcp.CallToolResult
	Pending                *gateway.Call
	descriptor, definition map[string]any
	allFields              []Field
	workflow               []any
	step                   int
	results                map[string]any
	hidden                 map[string]bool
	used                   map[string]string
	pendingKind            string
	pendingSource          map[string]any
	reviewHash             string
	confirmation           map[string]any
	callName               string
	callArguments          map[string]any
	wrote                  bool
}

func New(c gateway.Catalog, alias string) (*Run, error) {
	d := c.Definitions[alias]
	if d == nil || d["availability"] == "unavailable" {
		return nil, gateway.ErrAccess
	}
	origin := object(object(object(d["tool"])["_meta"])["nox/gateway"])
	if text(origin["productId"]) == "" || text(origin["serverId"]) == "" {
		return nil, errors.New("The tool is missing its Gateway product/server identity.")
	}
	if text(d["fingerprint"]) == "" {
		return nil, gateway.ErrStale
	}
	r := &Run{Catalog: c, Alias: alias, Arguments: map[string]any{}, definition: d, results: map[string]any{}, hidden: map[string]bool{}, used: map[string]string{alias: text(d["fingerprint"])}}
	if raw, ok := d["cli"]; ok {
		cli, e := noxmcp.InterpretCLI(object(raw))
		if e != nil {
			return nil, errors.New("Unsupported or invalid CLI descriptor.")
		}
		r.descriptor = cli
	}
	if _, e := compile(object(object(d["tool"])["inputSchema"])); e != nil {
		return nil, errors.New("The input schema cannot be validated locally.")
	}
	var check func(any) error
	check = func(v any) error {
		switch x := v.(type) {
		case map[string]any:
			if target, ok := x["tool"].(string); ok {
				if _, e := r.reference(target); e != nil {
					return e
				}
			}
			for _, child := range x {
				if e := check(child); e != nil {
					return e
				}
			}
		case []any:
			for _, child := range x {
				if e := check(child); e != nil {
					return e
				}
			}
		}
		return nil
	}
	// Only schema-defined references are followed; literal argument objects may
	// legitimately contain a field named "tool" and are not references.
	for _, p := range object(r.descriptor["parameters"]) {
		if source := object(object(p)["options"]); source != nil {
			if e := check(map[string]any{"tool": source["tool"]}); e != nil {
				return nil, e
			}
			if !r.readOnly(text(source["tool"])) {
				return nil, errors.New("Automatic selectors require a read-only tool.")
			}
		}
	}
	if preview := object(r.descriptor["preview"]); preview != nil {
		if _, e := r.reference(text(preview["tool"])); e != nil {
			return nil, e
		}
		if !r.readOnly(text(preview["tool"])) {
			return nil, errors.New("Automatic previews require a read-only tool.")
		}
		for path := range object(preview["bind"]) {
			r.hidden[path] = true
		}
	}
	r.confirmation = object(r.descriptor["confirmation"])
	if path := text(r.confirmation["bindPointer"]); path != "" {
		r.hidden[path] = true
	}
	for _, v := range array(r.descriptor["workflow"]) {
		if p := text(object(object(v)["confirmation"])["bindPointer"]); p != "" {
			r.hidden[p] = true
		}
	}
	fields, e := fieldsFor(object(object(d["tool"])["inputSchema"]), object(r.descriptor["parameters"]), r.hidden)
	if e != nil {
		return nil, e
	}
	r.allFields = fields
	r.workflow = array(r.descriptor["workflow"])
	for _, v := range r.workflow {
		s := object(v)
		if s["type"] == "call" {
			if _, e := r.reference(text(s["tool"])); e != nil {
				return nil, e
			}
		}
		if p := text(object(s["confirmation"])["bindPointer"]); p != "" {
			r.hidden[p] = true
		}
	}
	if len(r.workflow) > 0 {
		r.advanceWorkflow()
	} else {
		r.startInput(fields)
	}
	return r, nil
}
func (r *Run) reference(name string) (map[string]any, error) {
	target := r.Catalog.Definitions[name]
	if target == nil || target["availability"] == "unavailable" {
		return nil, errors.New("A referenced tool is missing or unavailable.")
	}
	origin := object(object(object(r.definition["tool"])["_meta"])["nox/gateway"])
	other := object(object(object(target["tool"])["_meta"])["nox/gateway"])
	if text(origin["productId"]) == "" || origin["productId"] != other["productId"] || origin["serverId"] != other["serverId"] {
		return nil, errors.New("An auxiliary reference is outside the originating product/server.")
	}
	if text(target["fingerprint"]) == "" {
		return nil, gateway.ErrStale
	}
	r.used[name] = text(target["fingerprint"])
	return target, nil
}
func (r *Run) readOnly(name string) bool {
	return object(object(r.Catalog.Definitions[name]["tool"])["annotations"])["readOnlyHint"] == true
}
func (r *Run) Current() *Field {
	if r.Index < 0 || r.Index >= len(r.Fields) {
		return nil
	}
	return &r.Fields[r.Index]
}
func (r *Run) Required(f Field) bool {
	if !f.Required {
		return false
	}
	if f.AncestorsRequired {
		return true
	}
	parent := f.Pointer[:strings.LastIndex(f.Pointer, "/")]
	_, ok := Get(r.Arguments, parent)
	return ok
}
func (r *Run) startInput(fields []Field) { r.Fields = fields; r.Index = 0; r.prepareField() }
func (r *Run) prepareField() {
	if r.Index >= len(r.Fields) {
		if len(r.workflow) > 0 {
			r.step++
			r.advanceWorkflow()
		} else {
			r.afterInput()
		}
		return
	}
	f := r.Fields[r.Index]
	r.Options = append([]Option{}, f.Static...)
	r.Mode = "input"
	if f.Options != nil {
		args, e := r.bind(f.Options)
		if e != nil {
			r.fail(e)
			return
		}
		r.request(text(f.Options["tool"]), args, "options", f.Options, false)
	}
}
func (r *Run) Set(value any, present bool) error {
	if r.Mode != "input" {
		return errors.New("No input is active.")
	}
	f := r.Current()
	if f == nil {
		return errors.New("No input is active.")
	}
	if !present && r.Required(*f) {
		return errors.New("This field is required.")
	}
	if present && (f.Control == "select" || f.Control == "multiselect") {
		if r.Required(*f) && len(r.Options) == 0 {
			return errors.New("No options are available for this required field.")
		}
		values := []any{value}
		if f.Control == "multiselect" {
			var ok bool
			values, ok = value.([]any)
			if !ok {
				return errors.New("Select an array of option values.")
			}
		}
		for _, v := range values {
			found := false
			for _, o := range r.Options {
				if Equal(o.Value, v) {
					found = true
					break
				}
			}
			if !found {
				return errors.New("Choose a value from the published options.")
			}
		}
	}
	if present {
		schema := clone(f.Schema).(map[string]any)
		root := object(object(r.definition["tool"])["inputSchema"])
		for _, key := range []string{"$defs", "definitions"} {
			if root[key] != nil {
				schema[key] = root[key]
			}
		}
		if e := Validate(schema, value); e != nil {
			return e
		}
	}
	old, exists := Get(r.Arguments, f.Pointer)
	if exists != present || !Equal(old, value) {
		for i := r.Index + 1; i < len(r.Fields); i++ {
			_ = Put(r.Arguments, r.Fields[i].Pointer, nil, false)
		}
		for path := range r.hidden {
			_ = Put(r.Arguments, path, nil, false)
		}
		r.reviewHash = ""
		r.Preview = nil
	}
	if f.Secret && present {
		if s, ok := value.(string); ok {
			r.secrets = append(r.secrets, s)
		} else {
			r.secrets = append(r.secrets, Pretty(value))
		}
	}
	if e := Put(r.Arguments, f.Pointer, value, present); e != nil {
		return e
	}
	r.Index++
	r.prepareField()
	return nil
}
func (r *Run) Back() {
	if r.Mode == "input" && r.Index > 0 {
		r.Index--
		r.prepareField()
	}
}
func (r *Run) Edit() error {
	if r.wrote {
		return errors.New("Earlier workflow writes cannot be replayed. Start a new action after inspecting the app.")
	}
	r.reviewHash = ""
	r.Preview = nil
	for path := range r.hidden {
		_ = Put(r.Arguments, path, nil, false)
	}
	if len(r.workflow) > 0 {
		r.step = 0
		r.results = map[string]any{}
		r.advanceWorkflow()
	} else {
		r.startInput(r.Fields)
	}
	return nil
}
func (r *Run) afterInput() {
	if source := object(r.descriptor["preview"]); source != nil {
		args, e := r.bind(source)
		if e != nil {
			r.fail(e)
			return
		}
		r.request(text(source["tool"]), args, "preview", source, false)
		return
	}
	r.review(r.Alias, r.Arguments, object(r.descriptor["confirmation"]))
}
func (r *Run) review(name string, args map[string]any, confirmation map[string]any) {
	r.Mode = "review"
	r.callName = name
	r.callArguments = clone(args).(map[string]any)
	r.confirmation = confirmation
	r.reviewHash = r.hash()
}
func (r *Run) hash() string {
	b, _ := json.Marshal(map[string]any{"arguments": r.Arguments, "callArguments": r.callArguments, "preview": r.Preview, "definitions": r.used, "step": r.step})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (r *Run) Approve(typed string) error {
	if r.Mode != "review" || r.reviewHash == "" || r.reviewHash != r.hash() {
		return errors.New("The review changed. Review the action again.")
	}
	if r.confirmation["mode"] == "typed" {
		expected, ok := Get(r.Arguments, text(r.confirmation["matchPointer"]))
		if !ok {
			return errors.New("The confirmation value is missing.")
		}
		match := text(expected)
		if _, ok := expected.(string); !ok {
			encoded, _ := json.Marshal(expected)
			match = string(encoded)
		}
		if typed != match {
			return errors.New("The confirmation text does not match.")
		}
	}
	if path, exists := r.confirmation["bindPointer"].(string); exists {
		if e := Put(r.Arguments, path, true, true); e != nil {
			return e
		}
		if r.callName == r.Alias {
			r.callArguments = clone(r.Arguments).(map[string]any)
		}
	}
	if len(r.workflow) > 0 && object(r.workflow[r.step])["type"] == "confirm" {
		r.reviewHash = ""
		r.step++
		r.advanceWorkflow()
		return nil
	}
	r.request(r.callName, r.callArguments, "execute", nil, true)
	return nil
}
func (r *Run) bind(source map[string]any) (map[string]any, error) {
	args := map[string]any{}
	if source["arguments"] == nil {
		for _, v := range array(source["input"]) {
			p := text(v)
			if value, ok := Get(r.Arguments, p); ok {
				if e := Put(args, p, value, true); e != nil {
					return nil, e
				}
			}
		}
		return args, nil
	}
	for name, v := range object(source["arguments"]) {
		b := object(v)
		value, ok := b["value"]
		if !ok {
			root := any(r.Arguments)
			if b["from"] == "step" {
				root = r.results[text(b["step"])]
			}
			value, ok = Get(root, text(b["pointer"]))
		}
		if !ok {
			continue
		}
		args[name] = clone(value)
	}
	return args, nil
}
func (r *Run) request(name string, args map[string]any, kind string, source map[string]any, revalidate bool) {
	definition, e := r.reference(name)
	if e != nil {
		r.fail(e)
		return
	}
	if e := Validate(object(definition["tool"])["inputSchema"], args); e != nil {
		r.fail(e)
		return
	}
	r.Pending = &gateway.Call{Catalog: r.Catalog, Name: name, Fingerprint: text(definition["fingerprint"]), Arguments: clone(args).(map[string]any), ReadOnly: r.readOnly(name), Expected: map[string]string{}, Revalidate: revalidate}
	for n, f := range r.used {
		r.Pending.Expected[n] = f
	}
	r.pendingKind = kind
	r.pendingSource = source
	r.Mode = "pending"
}
func (r *Run) Receive(result *mcp.CallToolResult, err error) {
	pending := r.Pending
	r.Pending = nil
	if pending == nil {
		return
	}
	if !pending.ReadOnly {
		r.wrote = true
		r.Catalog.LoadedAt = time.Time{}
	}
	r.Result = result
	if err != nil {
		r.fail(err)
		return
	}
	if result == nil {
		r.fail(gateway.ErrProtocol)
		return
	}
	if result.IsError {
		r.fail(errors.New("The tool reported an error. No later workflow steps were executed."))
		return
	}
	if result.NeedsInput() {
		r.fail(errors.New("The tool requested an unsupported multi-round-trip interaction. It was not replayed."))
		return
	}
	if schema := object(r.Catalog.Definitions[pending.Name]["tool"])["outputSchema"]; schema != nil {
		if e := Validate(schema, result.StructuredContent); e != nil {
			r.fail(errors.New("The tool returned data outside its output schema. Inspect the result before trying again."))
			return
		}
	}
	switch r.pendingKind {
	case "options":
		choices, e := parseOptions(r.pendingSource, result.StructuredContent)
		if e != nil {
			r.fail(e)
			return
		}
		r.Options = choices
		r.Mode = "input"
	case "preview":
		r.Preview = result.StructuredContent
		if pointer, ok := r.pendingSource["reviewPointer"].(string); ok {
			v, found := Get(result.StructuredContent, pointer)
			if !found {
				r.fail(errors.New("The preview review data is missing."))
				return
			}
			r.Preview = v
		}
		for path, v := range object(r.pendingSource["bind"]) {
			value, ok := Get(result.StructuredContent, text(v))
			if !ok {
				r.fail(errors.New("A required preview binding is missing."))
				return
			}
			if e := Put(r.Arguments, path, value, true); e != nil {
				r.fail(e)
				return
			}
		}
		r.review(r.Alias, r.Arguments, object(r.descriptor["confirmation"]))
	case "execute":
		r.reviewHash = ""
		if len(r.workflow) > 0 {
			step := object(r.workflow[r.step])
			r.results[text(step["id"])] = clone(result.StructuredContent)
			r.step++
			r.advanceWorkflow()
		} else {
			r.Mode = "done"
		}
	}
}
func (r *Run) advanceWorkflow() {
	if r.step >= len(r.workflow) {
		r.Mode = "done"
		return
	}
	step := object(r.workflow[r.step])
	switch step["type"] {
	case "input":
		selected := []Field{}
		for _, p := range array(step["parameters"]) {
			found := false
			for _, f := range r.allFields {
				if f.Pointer == p {
					selected = append(selected, f)
					found = true
				}
			}
			if !found {
				r.fail(errors.New("The workflow references an unsupported input pointer."))
				return
			}
		}
		r.startInput(selected)
	case "confirm":
		r.Preview = r.results
		r.review("", map[string]any{}, object(step["confirmation"]))
	case "call":
		args, e := r.bind(step)
		if e != nil {
			r.fail(e)
			return
		}
		name := text(step["tool"])
		if r.readOnly(name) {
			r.request(name, args, "execute", nil, false)
		} else {
			r.review(name, args, nil)
		}
	default:
		r.fail(errors.New("Unsupported workflow step."))
	}
}
func (r *Run) fail(e error) { r.Mode = "error"; r.Problem = e.Error(); r.reviewHash = "" }
func (r *Run) SecretValues() []string {
	values := append([]string{}, r.secrets...)
	for _, f := range r.allFields {
		if f.Secret {
			if v, ok := Get(r.Arguments, f.Pointer); ok {
				if s, ok := v.(string); ok && s != "" {
					values = append(values, s)
				}
			}
		}
	}
	return values
}
func (r *Run) ReviewArguments() any {
	value := clone(r.Arguments)
	for _, f := range r.allFields {
		if f.Secret {
			if _, ok := Get(value, f.Pointer); ok {
				_ = Put(object(value), f.Pointer, "[hidden]", true)
			}
		}
	}
	return value
}
func (r *Run) ConfirmationMessage() string { return text(r.confirmation["message"]) }
func (r *Run) TypedConfirmation() bool     { return r.confirmation["mode"] == "typed" }
func (r *Run) Target() string {
	if r.callName != "" {
		return r.callName
	}
	return fmt.Sprintf("%s · step %d", r.Alias, r.step+1)
}

// DirectRead reports whether all requested input has been collected for a
// read that needs no explicit review. Normal guarded dispatch still applies.
func (r *Run) DirectRead() bool {
	annotations := object(object(r.definition["tool"])["annotations"])
	return r.Mode == "review" && len(r.workflow) == 0 &&
		object(r.descriptor["preview"]) == nil && len(r.confirmation) == 0 &&
		annotations["readOnlyHint"] == true && annotations["destructiveHint"] != true
}
