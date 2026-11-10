package maintenance

import (
	"context"
	"github.com/mapherez/nox-cli/internal/releases"
	"os"
	"path/filepath"
	"testing"
)

type source struct{ calls int }

func (s *source) Latest(context.Context) (releases.Release, error) {
	s.calls++
	return releases.Release{Version: "v1.1.0"}, nil
}
func (*source) Download(context.Context, string, string) (string, error) {
	panic("source build must not download an update")
}
func TestChecksAreBoundedAndSourceBuildsNeverSelfReplace(t *testing.T) {
	root := t.TempDir()
	remote := &source{}
	m := &Manager{Root: root, Version: "dev", Source: remote}
	for range 3 {
		result, err := m.Check(context.Background(), false)
		if err != nil || result.Latest != "v1.1.0" {
			t.Fatal(result, err)
		}
	}
	if remote.calls != 1 {
		t.Fatal("automatic release checks were not cached")
	}
	if _, err := m.Check(context.Background(), true); err != nil || remote.calls != 2 {
		t.Fatal("manual check failed")
	}
	if _, err := m.Update(context.Background(), "v1.1.0"); err == nil {
		t.Fatal("source build was replaced")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "update-check.json"))
	if len(raw) == 0 {
		t.Fatal("check record not written")
	}
}
