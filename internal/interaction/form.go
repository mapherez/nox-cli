package interaction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Field struct {
	Pointer, Label, Description, Control, Type string
	Required, AncestorsRequired, Secret        bool
	Schema, Options                            map[string]any
	Static                                     []Option
	Order                                      float64
}
type Option struct {
	Label string
	Value any
}

func schemaAt(root, raw map[string]any) (map[string]any, error) {
	for i := 0; i < 32; i++ {
		ref := text(raw["$ref"])
		if ref == "" {
			return raw, nil
		}
		if !strings.HasPrefix(ref, "#/") {
			return nil, errors.New("External schema references are not supported.")
		}
		v, ok := Get(root, ref[1:])
		if !ok || object(v) == nil {
			return nil, errors.New("Unresolved schema reference.")
		}
		raw = object(v)
	}
	return nil, errors.New("Recursive form schemas are not supported.")
}
func fieldsFor(root, parameters map[string]any, hidden map[string]bool) ([]Field, error) {
	fields := []Field{}
	var visit func(map[string]any, string, bool) error
	visit = func(raw map[string]any, pointer string, ancestors bool) error {
		if strings.Count(pointer, "/") > 32 {
			return errors.New("Recursive or deeply nested input forms are not supported.")
		}
		schema, e := schemaAt(root, raw)
		if e != nil {
			return e
		}
		properties := object(schema["properties"])
		if len(properties) == 0 {
			return nil
		}
		required := map[string]bool{}
		for _, name := range array(schema["required"]) {
			required[text(name)] = true
		}
		keys := []string{}
		for k := range properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, name := range keys {
			path := pointer + "/" + escape(name)
			if hidden[path] {
				continue
			}
			sub, e := schemaAt(root, object(properties[name]))
			if e != nil {
				return e
			}
			parameter := object(parameters[path])
			control := text(parameter["control"])
			if text(sub["type"]) == "object" && len(object(sub["properties"])) > 0 && control == "" && parameter["options"] == nil && sub["writeOnly"] != true {
				if e := visit(sub, path, ancestors && required[name]); e != nil {
					return e
				}
				continue
			}
			label := text(sub["title"])
			if label == "" {
				label = strings.TrimPrefix(path, "/")
			}
			f := Field{Pointer: path, Label: label, Description: text(sub["description"]), Required: required[name], AncestorsRequired: ancestors, Schema: sub, Options: object(parameter["options"]), Type: text(sub["type"]), Order: 0, Secret: sub["writeOnly"] == true || sub["format"] == "password"}
			f.Order, _ = parameter["order"].(float64)
			if f.Options != nil && control == "" {
				control = "select"
			}
			if control == "" {
				switch f.Type {
				case "boolean":
					control = "checkbox"
				case "integer", "number":
					control = "number"
				case "string":
					control = "text"
				default:
					control = "json"
				}
			}
			if f.Secret {
				control = "password"
			}
			f.Control = control
			f.Secret = f.Secret || control == "password"
			if control == "select" || control == "multiselect" || sub["enum"] != nil || sub["const"] != nil {
				values := array(sub["enum"])
				if control == "multiselect" {
					values = array(object(sub["items"])["enum"])
				}
				if v, ok := sub["const"]; ok {
					values = []any{v}
				}
				for _, v := range values {
					label := text(v)
					if _, ok := v.(string); !ok {
						label = Pretty(v)
					}
					f.Static = append(f.Static, Option{Label: label, Value: v})
				}
				if f.Options == nil && len(f.Static) > 0 && control != "multiselect" {
					f.Control = "select"
				}
			}
			fields = append(fields, f)
		}
		return nil
	}
	if e := visit(root, "", true); e != nil {
		return nil, e
	}
	if len(fields) == 0 && (root["oneOf"] != nil || root["anyOf"] != nil || root["allOf"] != nil || root["additionalProperties"] == true) {
		fields = append(fields, Field{Pointer: "", Label: "Arguments (JSON)", Control: "json", Required: true, AncestorsRequired: true, Schema: root})
	}
	present := map[string]bool{}
	for _, f := range fields {
		present[f.Pointer] = true
	}
	for path := range parameters {
		if !present[path] && !hidden[path] {
			return nil, fmt.Errorf("Unsupported parameter pointer: %s", path)
		}
	}
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Order != fields[j].Order {
			return fields[i].Order < fields[j].Order
		}
		return fields[i].Pointer < fields[j].Pointer
	})
	// Dependency order takes precedence over presentation order. Cycles block only
	// this action, rather than issuing selectors with guessed argument values.
	ordered := []Field{}
	done := map[string]bool{}
	for len(ordered) < len(fields) {
		progress := false
		for _, f := range fields {
			if done[f.Pointer] {
				continue
			}
			ready := true
			for _, dep := range inputDependencies(f.Options) {
				for _, other := range fields {
					if (other.Pointer == dep || strings.HasPrefix(dep, other.Pointer+"/")) && !done[other.Pointer] {
						ready = false
					}
				}
			}
			if ready {
				ordered = append(ordered, f)
				done[f.Pointer] = true
				progress = true
			}
		}
		if !progress {
			return nil, errors.New("Dynamic selectors have a circular dependency.")
		}
	}
	return ordered, nil
}
func inputDependencies(source map[string]any) []string {
	out := []string{}
	for _, v := range array(source["input"]) {
		out = append(out, text(v))
	}
	for _, v := range object(source["arguments"]) {
		b := object(v)
		if b["from"] == "input" {
			out = append(out, text(b["pointer"]))
		}
	}
	return out
}
func (f Field) Parse(raw string) (any, error) {
	if f.Type == "string" && (f.Control == "text" || f.Control == "password" || f.Control == "textarea") {
		return raw, nil
	}
	v, e := Decode(raw)
	if e != nil {
		return nil, e
	}
	return v, nil
}
func parseOptions(source map[string]any, data any) ([]Option, error) {
	items, ok := Get(data, text(source["itemsPointer"]))
	if !ok {
		return []Option{}, nil
	}
	list, ok := items.([]any)
	if !ok {
		return nil, errors.New("The options result must contain an array.")
	}
	if len(list) > 5000 {
		return nil, errors.New("This selector exceeds the 5,000-option display limit.")
	}
	options := []Option{}
	ids := map[string]bool{}
	labels := map[string]int{}
	for _, item := range list {
		value, ok := Get(item, text(source["valuePointer"]))
		if !ok {
			return nil, errors.New("An option is missing its declared value.")
		}
		id := Pretty(value)
		if ids[id] {
			return nil, errors.New("The selector returned duplicate option IDs.")
		}
		ids[id] = true
		label, ok := Get(item, text(source["labelPointer"]))
		if !ok {
			label = value
		}
		name := text(label)
		if _, ok := label.(string); !ok {
			name = Pretty(label)
		}
		labels[name]++
		options = append(options, Option{Label: name, Value: value})
	}
	for i := range options {
		if labels[options[i].Label] > 1 {
			options[i].Label += " [" + Pretty(options[i].Value) + "]"
		}
	}
	return options, nil
}
