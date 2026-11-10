package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mapherez/nox-cli/internal/credentials"
)

func TestUpdatePreservesConfigurationAndCanRecoverInterruptedCommit(t *testing.T) {
	root, source := staged(t)
	paths := &testPaths{}
	if err := activate(root, source, "v1.0.0", paths, missingCommand); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.Base(source))
	old, _ := os.ReadFile(target)
	before, _ := os.ReadFile(filepath.Join(root, "install-state.json"))
	var state ownership
	_ = json.Unmarshal(before, &state)
	_ = os.WriteFile(filepath.Join(root, "config.json"), []byte("settings-kept"), 0600)
	_ = os.WriteFile(source, []byte("updated-binary"), 0700)
	if err := Apply(context.Background(), root, source, "v1.1.0"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "updated-binary" {
		t.Fatal("update did not activate")
	}
	settings, _ := os.ReadFile(filepath.Join(root, "config.json"))
	if string(settings) != "settings-kept" {
		t.Fatal("configuration changed")
	}
	// Model power loss between executable replacement and ownership replacement.
	stage := filepath.Join(root, "install")
	journal := updateJournal{Schema: 1, Product: "nox-cli", Before: digest(old), After: digest(got), State: state}
	raw, _ := json.Marshal(journal)
	_ = os.WriteFile(filepath.Join(stage, "previous.bin"), old, 0600)
	_ = os.WriteFile(filepath.Join(stage, "previous-state.json"), before, 0600)
	_ = os.WriteFile(filepath.Join(stage, "update.json"), raw, 0600)
	if err := Recover(root); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(target)
	if string(restored) != string(old) {
		t.Fatal("interrupted update was not recovered")
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("owned staging was not cleaned")
	}
}
func TestUpdateRejectsModifiedOwnershipAndCancelledOperation(t *testing.T) {
	root, source := staged(t)
	if err := activate(root, source, "v1.0.0", &testPaths{}, missingCommand); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.Base(source))
	_ = os.WriteFile(target, []byte("foreign-modified"), 0700)
	if err := Apply(context.Background(), root, source, "v1.1.0"); err == nil {
		t.Fatal("modified executable replaced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Apply(ctx, root, source, "v1.1.0"); err == nil {
		t.Fatal("cancelled update executed")
	}
	if _, err := os.Stat(filepath.Join(root, "install", "update.json")); !os.IsNotExist(err) {
		t.Fatal("cancelled update wrote a journal")
	}
}

type removePaths struct{ testPaths }

func (p *removePaths) Removal(string, ownership) ([]pathEdit, error) { return nil, nil }

type removeVault struct{ deleted bool }

func (*removeVault) Get() (credentials.Secret, error) { panic("uninstall must not read secrets") }
func (*removeVault) Set(credentials.Secret) error     { panic("uninstall must not write secrets") }
func (v *removeVault) Delete() error                  { v.deleted = true; return nil }
func TestUninstallPreservesDataUnlessExplicitlyPurged(t *testing.T) {
	for _, purge := range []bool{false, true} {
		root, source := staged(t)
		p := &removePaths{}
		if err := activate(root, source, "v1.0.0", p, missingCommand); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(root, "config.json"), []byte("settings"), 0600)
		_ = os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("keep"), 0600)
		vault := &removeVault{}
		if err := remove(root, purge, p, vault); err != nil {
			t.Fatal(err)
		}
		if vault.deleted != purge {
			t.Fatal("unexpected vault deletion")
		}
		_, err := os.Stat(filepath.Join(root, "config.json"))
		if os.IsNotExist(err) != purge {
			t.Fatal("configuration retention mismatch")
		}
		if _, err := os.Stat(filepath.Join(root, "unrelated.txt")); err != nil {
			t.Fatal("unrelated file removed")
		}
		if _, err := os.Stat(filepath.Join(root, filepath.Base(source))); !os.IsNotExist(err) {
			t.Fatal("executable remains")
		}
	}
}

func TestExplicitPurgeAfterKeepSettingsUninstall(t *testing.T) {
	root, source := staged(t)
	paths := &removePaths{}
	if err := activate(root, source, "v1.0.0", paths, missingCommand); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "config.json"), []byte("settings"), 0600)
	vault := &removeVault{}
	if err := remove(root, false, paths, vault); err != nil {
		t.Fatal(err)
	}
	if err := remove(root, true, paths, vault); err != nil {
		t.Fatal(err)
	}
	if !vault.deleted {
		t.Fatal("explicit second uninstall did not purge owned credential")
	}
	if _, err := os.Stat(filepath.Join(root, "config.json")); !os.IsNotExist(err) {
		t.Fatal("configuration retained after purge")
	}
}
