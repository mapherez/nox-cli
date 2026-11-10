package install

import (
	"crypto/rand"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestWindowsPathPreservesRegistryTypeAndRollsBack(t *testing.T) {
	path := `Software\nox-cli-test-` + rand.Text()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	defer key.Close()
	if err := key.SetStringValue("Path", `C:\existing;%UNCHANGED_LITERAL%`); err != nil {
		t.Fatal(err)
	}
	p := windowsPaths{keyPath: path}
	edits, err := p.Prepare(`C:\user with spaces\nox-cli`)
	if err != nil || len(edits) != 1 {
		t.Fatal(err)
	}
	if err := p.Apply(edits[0], false); err != nil {
		t.Fatal(err)
	}
	_, kind, err := key.GetStringValue("Path")
	if err != nil || kind != registry.SZ {
		t.Fatal("registry type was changed", err)
	}
	if err := p.Apply(edits[0], true); err != nil {
		t.Fatal(err)
	}
	value, _, err := key.GetStringValue("Path")
	if err != nil || value != `C:\existing;%UNCHANGED_LITERAL%` {
		t.Fatal("unrelated user PATH was not restored", err)
	}
}

func TestWindowsRemovalPreservesUnrelatedUserPathEntries(t *testing.T) {
	path := `Software\nox-cli-test-` + rand.Text()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	defer key.Close()
	before := `C:\existing;C:\user with spaces\nox-cli;%UNCHANGED_LITERAL%`
	if err := key.SetStringValue("Path", before); err != nil {
		t.Fatal(err)
	}
	p := windowsPaths{keyPath: path}
	edits, err := p.Removal(`C:\user with spaces\nox-cli`, ownership{Hooks: []hookRecord{{Target: "HKCU/Environment/Path", Added: true}}})
	if err != nil || len(edits) != 1 {
		t.Fatal(err)
	}
	if err := p.Apply(edits[0], false); err != nil {
		t.Fatal(err)
	}
	actual, kind, err := key.GetStringValue("Path")
	if err != nil || kind != registry.SZ || actual != `C:\existing;%UNCHANGED_LITERAL%` {
		t.Fatal("unrelated PATH entries or registry type changed", err)
	}
}
