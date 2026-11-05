package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type testPaths struct {
	value []byte
	fail  bool
}

func (p *testPaths) Prepare(string) ([]pathEdit, error) {
	after := []byte("managed path")
	return []pathEdit{{Target: "test-profile", Before: p.value, After: after, Existed: true, Mode: 0600}}, nil
}
func (p *testPaths) Apply(e pathEdit, reverse bool) error {
	if p.fail {
		return ErrInstall
	}
	if reverse {
		p.value = e.Before
	} else {
		p.value = e.After
	}
	return nil
}
func staged(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "user with spaces", "nox-cli")
	stage := filepath.Join(root, "install")
	if err := os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	name := "nox"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	source := filepath.Join(stage, name)
	if err := os.WriteFile(source, []byte("verified-test-payload"), 0700); err != nil {
		t.Fatal(err)
	}
	return root, source
}
func missingCommand(string) (string, error) { return "", os.ErrNotExist }

func TestActivationReinstallAndOwnership(t *testing.T) {
	root, source := staged(t)
	paths := &testPaths{value: []byte("unrelated existing PATH")}
	for range 2 {
		if err := activate(root, source, "test", paths, missingCommand); err != nil {
			t.Fatal(err)
		}
	}
	payload, _ := os.ReadFile(filepath.Join(root, filepath.Base(source)))
	if string(payload) != "verified-test-payload" {
		t.Fatal("wrong installed binary")
	}
	var state ownership
	data, _ := os.ReadFile(filepath.Join(root, "install-state.json"))
	if json.Unmarshal(data, &state) != nil || state.Product != "nox-cli" || len(state.Hooks) != 1 || !state.Hooks[0].Added {
		t.Fatal("reinstall lost original PATH ownership")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "install"))
	if len(entries) != 1 || entries[0].Name() != filepath.Base(source) {
		t.Fatal("activation staging files remain")
	}
}

func TestActivationRejectsForeignCommandAndRollsBackFailure(t *testing.T) {
	root, source := staged(t)
	paths := &testPaths{value: []byte("original")}
	err := activate(root, source, "test", paths, func(string) (string, error) { return filepath.Join(t.TempDir(), "nox"), nil })
	if err == nil {
		t.Fatal("foreign nox command overwritten")
	}
	if _, err := os.Stat(filepath.Join(root, "install-state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("ownership written before collision check")
	}
	paths.fail = true
	if err := activate(root, source, "test", paths, missingCommand); err == nil {
		t.Fatal("failed PATH change reported success")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.Base(source))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed installation left executable active")
	}
	if string(paths.value) != "original" {
		t.Fatal("unrelated PATH data changed")
	}
	if err := os.WriteFile(filepath.Join(root, filepath.Base(source)), []byte("unowned"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := activate(root, source, "test", paths, missingCommand); err == nil {
		t.Fatal("unowned target was claimed")
	}
}
