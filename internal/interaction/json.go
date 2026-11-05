// Package interaction interprets published CLI descriptors without product-specific rules.
package interaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func text(v any) string           { s, _ := v.(string); return s }
func array(v any) []any           { a, _ := v.([]any); return a }
func clone(v any) any             { b, _ := json.Marshal(v); x, _ := Decode(string(b)); return x }
func Decode(raw string) (any, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, errors.New("Enter valid JSON.")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("Enter one JSON value.")
	}
	return v, nil
}
func Pretty(v any) string {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return "[unavailable]"
	}
	return string(b)
}
func Equal(a, b any) bool    { x, _ := json.Marshal(a); y, _ := json.Marshal(b); return bytes.Equal(x, y) }
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func segments(p string) ([]string, error) {
	if p == "" {
		return nil, nil
	}
	if !strings.HasPrefix(p, "/") {
		return nil, errors.New("Invalid JSON pointer.")
	}
	out := strings.Split(p[1:], "/")
	for i, s := range out {
		for j := 0; j < len(s); j++ {
			if s[j] == '~' {
				if j+1 == len(s) || (s[j+1] != '0' && s[j+1] != '1') {
					return nil, errors.New("Invalid JSON pointer.")
				}
				j++
			}
		}
		out[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
	}
	return out, nil
}
func Get(v any, p string) (any, bool) {
	parts, e := segments(p)
	if e != nil {
		return nil, false
	}
	for _, s := range parts {
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = x[s]
			if !ok {
				return nil, false
			}
		case []any:
			i, e := strconv.Atoi(s)
			if e != nil || i < 0 || i >= len(x) || strconv.Itoa(i) != s {
				return nil, false
			}
			v = x[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func Put(root map[string]any, p string, value any, present bool) error {
	parts, e := segments(p)
	if e != nil {
		return e
	}
	if len(parts) == 0 {
		if !present {
			clear(root)
			return nil
		}
		m := object(clone(value))
		if m == nil {
			return errors.New("Arguments must be a JSON object.")
		}
		clear(root)
		for k, v := range m {
			root[k] = v
		}
		return nil
	}
	_, e = setPointer(root, parts, value, present)
	return e
}
func setPointer(node any, parts []string, value any, present bool) (any, error) {
	if len(parts) == 0 {
		if present {
			return clone(value), nil
		}
		return nil, nil
	}
	switch x := node.(type) {
	case map[string]any:
		key := parts[0]
		if len(parts) == 1 {
			if present {
				x[key] = clone(value)
			} else {
				delete(x, key)
			}
			return x, nil
		}
		next, exists := x[key]
		if !exists {
			if !present {
				return x, nil
			}
			next = map[string]any{}
		}
		updated, e := setPointer(next, parts[1:], value, present)
		if e != nil {
			return nil, e
		}
		if !present && object(updated) != nil && len(object(updated)) == 0 {
			delete(x, key)
		} else {
			x[key] = updated
		}
		return x, nil
	case []any:
		i, e := strconv.Atoi(parts[0])
		if e != nil || i < 0 || strconv.Itoa(i) != parts[0] || i > len(x) {
			return nil, errors.New("Unsupported array pointer index.")
		}
		if i == len(x) {
			if !present {
				return x, nil
			}
			if len(parts) == 1 {
				return append(x, clone(value)), nil
			}
			x = append(x, map[string]any{})
		}
		if len(parts) == 1 {
			if present {
				x[i] = clone(value)
			} else {
				x = append(x[:i], x[i+1:]...)
			}
			return x, nil
		}
		updated, e := setPointer(x[i], parts[1:], value, present)
		if e != nil {
			return nil, e
		}
		x[i] = updated
		return x, nil
	default:
		return nil, errors.New("A JSON pointer crosses a non-container value.")
	}
}
func compile(raw any) (*jsonschema.Resolved, error) {
	b, e := json.Marshal(raw)
	if e != nil {
		return nil, e
	}
	var schema jsonschema.Schema
	if json.Unmarshal(b, &schema) != nil {
		return nil, errors.New("Unsupported JSON schema.")
	}
	return schema.Resolve(nil)
}
func Validate(raw, value any) error {
	resolved, e := compile(raw)
	if e != nil {
		return errors.New("This schema cannot be validated locally; external references are not loaded.")
	}
	validationValue, e := validationNumbers(value)
	if e != nil {
		return e
	}
	if resolved.Validate(validationValue) != nil {
		return errors.New("The value does not match the published schema. Check required fields, types and limits.")
	}
	return nil
}

// The schema library treats json.Number as a string for type checks. Supply a
// native-number validation copy, keeping original lexemes in arguments/results.
// Never round a large integer to fit the validator.
func validationNumbers(v any) (any, error) {
	switch x := v.(type) {
	case json.Number:
		if len(x.String()) > 1024 {
			return nil, errors.New("Number exceeds the supported validation range.")
		}
		if i := strings.IndexAny(x.String(), "eE"); i >= 0 {
			exponent, e := strconv.Atoi(x.String()[i+1:])
			if e != nil || exponent > 400 || exponent < -400 {
				return nil, errors.New("Number exceeds the supported validation range.")
			}
		}
		rational, ok := new(big.Rat).SetString(x.String())
		if !ok {
			return nil, errors.New("Unsupported numeric value.")
		}
		if rational.IsInt() {
			n := rational.Num()
			if n.IsInt64() {
				return n.Int64(), nil
			}
			if n.IsUint64() {
				return n.Uint64(), nil
			}
			return nil, errors.New("Integer exceeds the supported 64-bit validation range.")
		}
		number, e := x.Float64()
		if e != nil || math.IsInf(number, 0) || math.IsNaN(number) || (number == 0 && rational.Sign() != 0) {
			return nil, errors.New("Number exceeds the supported validation range.")
		}
		return number, nil
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			n, e := validationNumbers(val)
			if e != nil {
				return nil, e
			}
			out[k] = n
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			n, e := validationNumbers(val)
			if e != nil {
				return nil, e
			}
			out[i] = n
		}
		return out, nil
	}
	return v, nil
}
