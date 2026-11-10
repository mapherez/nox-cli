package interaction

import "testing"

func TestDirectReadWaitsForInputsAndHonorsPublishedInteractions(t *testing.T) {
	for _, test := range []struct {
		name                             string
		readonly, destructive, parameter bool
		descriptor                       map[string]any
		direct                           bool
	}{
		{name: "plain read", readonly: true, direct: true},
		{name: "write"},
		{name: "conflicting hints", readonly: true, destructive: true},
		{name: "input", readonly: true, parameter: true},
		{name: "confirmation", readonly: true, descriptor: map[string]any{"version": "1.0", "confirmation": map[string]any{"mode": "boolean"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := schema(map[string]any{})
			if test.parameter {
				input = schema(map[string]any{"filter": map[string]any{"type": "string"}})
			}
			d := definition("query", input, test.descriptor, test.readonly)
			if test.destructive {
				d["tool"].(map[string]any)["annotations"].(map[string]any)["destructiveHint"] = true
			}
			r, err := New(catalog(d), "query")
			if err != nil {
				t.Fatal(err)
			}
			if r.DirectRead() != test.direct {
				t.Fatalf("unexpected shortcut: %t", r.DirectRead())
			}
			if test.parameter {
				mustSet(t, r, "selected", true)
				if !r.DirectRead() {
					t.Fatal("completed read input still requires review")
				}
			}
		})
	}
}
