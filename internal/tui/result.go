package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/mapherez/nox-cli/internal/interaction"
)

// JSON rendering is presentation only; text content never supplies call bindings.
func readableResult(value any, schema any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "Result data could not be displayed."
	}
	normalized, err := interaction.Decode(string(raw))
	if err != nil {
		return "Result data could not be displayed."
	}
	lines := []string{}
	var visit func(any, map[string]any, string, int)
	visit = func(v any, s map[string]any, indent string, depth int) {
		if depth > 64 {
			lines = append(lines, indent+"Further nested data is not expanded.")
			return
		}
		switch x := v.(type) {
		case map[string]any:
			if len(x) == 0 {
				lines = append(lines, indent+"No fields.")
				return
			}
			keys := make([]string, 0, len(x))
			for key := range x {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			properties, _ := s["properties"].(map[string]any)
			labels := map[string]string{}
			counts := map[string]int{}
			for _, key := range keys {
				property, _ := properties[key].(map[string]any)
				label, _ := property["title"].(string)
				if label == "" {
					label = fieldLabel(key)
				}
				label = safeText(label)
				labels[key] = label
				counts[label]++
			}
			for _, key := range keys {
				label := labels[key]
				if counts[label] > 1 {
					label += " (" + safeText(key) + ")"
				}
				property, _ := properties[key].(map[string]any)
				switch x[key].(type) {
				case map[string]any, []any:
					lines = append(lines, indent+label)
					visit(x[key], property, indent+"  ", depth+1)
				default:
					value := strings.Split(resultScalar(x[key]), "\n")
					lines = append(lines, indent+label+": "+value[0])
					for _, line := range value[1:] {
						lines = append(lines, indent+"  "+line)
					}
				}
			}
		case []any:
			if len(x) == 0 {
				lines = append(lines, indent+"No items.")
				return
			}
			itemSchema, _ := s["items"].(map[string]any)
			for i, item := range x {
				switch item.(type) {
				case map[string]any, []any:
					lines = append(lines, fmt.Sprintf("%s%d.", indent, i+1))
					visit(item, itemSchema, indent+"  ", depth+1)
				default:
					lines = append(lines, indent+"• "+resultScalar(item))
				}
			}
		default:
			lines = append(lines, indent+resultScalar(x))
		}
	}
	shape, _ := schema.(map[string]any)
	visit(normalized, shape, "", 0)
	return strings.Join(lines, "\n")
}
func resultScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "Not set"
	case bool:
		if x {
			return "Yes"
		}
		return "No"
	case string:
		if x == "" {
			return "(empty)"
		}
		return x
	case json.Number:
		return x.String()
	default:
		return interaction.Pretty(v)
	}
}
func fieldLabel(key string) string {
	runes := []rune(safeText(key))
	var words strings.Builder
	for i, r := range runes {
		if r == '_' || r == '-' {
			words.WriteRune(' ')
			continue
		}
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) || (i+1 < len(runes) && unicode.IsUpper(runes[i-1]) && unicode.IsLower(runes[i+1]))) {
			words.WriteRune(' ')
		}
		words.WriteRune(r)
	}
	parts := strings.Fields(words.String())
	for i, word := range parts {
		if strings.EqualFold(word, "id") || strings.EqualFold(word, "url") || strings.EqualFold(word, "api") || strings.EqualFold(word, "http") || strings.EqualFold(word, "https") {
			parts[i] = strings.ToUpper(word)
			continue
		}
		letters := []rune(word)
		letters[0] = unicode.ToUpper(letters[0])
		parts[i] = string(letters)
	}
	if len(parts) == 0 {
		return "Field"
	}
	return strings.Join(parts, " ")
}
