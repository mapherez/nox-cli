package command

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestProcessOptionsAndNonInteractiveInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"version", []string{"--version"}, 0, "nox dev (commit unknown, built unknown)"},
		{"help", []string{"--help"}, 0, "Usage: nox"},
		{"no-terminal", nil, 2, "An interactive terminal is required"},
		{"unknown-secret", []string{"private-test-value"}, 2, "Unsupported arguments"},
		{"unexpected-extra", []string{"--version", "private-test-value"}, 2, "Unsupported arguments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			code := Run(context.Background(), tc.args, strings.NewReader(""), &out, &diagnostics)
			if code != tc.code || !strings.Contains(out.String()+diagnostics.String(), tc.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &diagnostics)
			}
			if tc.code == 0 && diagnostics.Len() != 0 || tc.code != 0 && out.Len() != 0 {
				t.Fatal("process output used the wrong stream")
			}
			if strings.Contains(out.String()+diagnostics.String(), "private-test-value") {
				t.Fatal("unexpected argument was exposed")
			}
		})
	}
}
