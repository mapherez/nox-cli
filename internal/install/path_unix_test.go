//go:build !windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellPathHooksAreIdempotentAndPreserveExistingProfiles(t *testing.T) {
	root, _ := staged(t)
	home := t.TempDir()
	original := "export UNRELATED=preserve-me\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	paths := &shellPaths{home: home, shell: "zsh"}
	for range 2 {
		edits, err := paths.Prepare(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, edit := range edits {
			if err := paths.Apply(edit, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if !strings.HasPrefix(string(data), original) || strings.Count(string(data), pathStart) != 1 {
		t.Fatal("profile was overwritten or PATH block duplicated")
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(strings.Replace(string(data), "export PATH=", "export MODIFIED_PATH=", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := paths.Prepare(root); err == nil {
		t.Fatal("modified PATH block could be silently replaced")
	}
}

func TestBashDoesNotHideExistingLoginProfile(t *testing.T) {
	root, _ := staged(t)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("keep-existing-login\n"), 0600); err != nil {
		t.Fatal(err)
	}
	edits, err := (&shellPaths{home: home, shell: "bash"}).Prepare(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 2 || edits[1].Target != filepath.Join(home, ".profile") {
		t.Fatal("new bash profile would shadow existing login setup")
	}
}

type failSecondPathWrite struct {
	inner  *shellPaths
	writes int
}

func (p *failSecondPathWrite) Prepare(root string) ([]pathEdit, error) { return p.inner.Prepare(root) }
func (p *failSecondPathWrite) Apply(edit pathEdit, reverse bool) error {
	if !reverse {
		p.writes++
		if p.writes == 2 {
			return ErrInstall
		}
	}
	return p.inner.Apply(edit, reverse)
}

func TestPartialShellActivationRestoresActualProfile(t *testing.T) {
	root, source := staged(t)
	home := t.TempDir()
	original := []byte("export UNRELATED=keep-this\n")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), original, 0600); err != nil {
		t.Fatal(err)
	}
	paths := &failSecondPathWrite{inner: &shellPaths{home: home, shell: "zsh"}}
	if err := activate(root, source, "test", paths, missingCommand); err == nil {
		t.Fatal("second profile failure reported success")
	}
	restored, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil || string(restored) != string(original) {
		t.Fatal("first profile was not restored", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zprofile")); !os.IsNotExist(err) {
		t.Fatal("failed new profile was left behind")
	}
	if _, err := os.Stat(filepath.Join(root, "install-state.json")); !os.IsNotExist(err) {
		t.Fatal("failed activation kept ownership state")
	}
}

func TestUninstallRemovesOnlyUnmodifiedOwnedProfileBlocks(t *testing.T) {
	root, source := staged(t)
	home := t.TempDir()
	profile := filepath.Join(home, ".zshrc")
	original := "export KEEP_ME=yes\n"
	_ = os.WriteFile(profile, []byte(original), 0600)
	p := &shellPaths{home: home, shell: "zsh"}
	if err := activate(root, source, "v1.0.0", p, missingCommand); err != nil {
		t.Fatal(err)
	}
	if err := remove(root, false, p, &removeVault{}); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(profile)
	if string(actual) != original {
		t.Fatal("unrelated profile contents changed")
	}
	root, source = staged(t)
	p = &shellPaths{home: home, shell: "zsh"}
	if err := activate(root, source, "v1.0.0", p, missingCommand); err != nil {
		t.Fatal(err)
	}
	changed, _ := os.ReadFile(profile)
	_ = os.WriteFile(profile, []byte(strings.Replace(string(changed), "export PATH=", "export USER_EDIT=", 1)), 0600)
	if err := remove(root, false, p, &removeVault{}); err == nil {
		t.Fatal("modified PATH block removed")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.Base(source))); err != nil {
		t.Fatal("failed preflight removed executable")
	}
}

func TestLoginAndInteractiveProfilesDoNotDuplicatePath(t *testing.T) {
	root, _ := staged(t)
	home := t.TempDir()
	p := &shellPaths{home: home, shell: "zsh"}
	edits, err := p.Prepare(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range edits {
		if err := p.Apply(edit, false); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", "-c", `. "$1"; . "$2"; printf '%s' "$PATH"`, "path-test", filepath.Join(home, ".zprofile"), filepath.Join(home, ".zshrc"))
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), root) != 1 {
		t.Fatal("profile loading duplicated the managed PATH entry")
	}
}

func TestReinstallRetainsOwnershipWhenChangingShell(t *testing.T) {
	root, source := staged(t)
	home := t.TempDir()
	if err := activate(root, source, "v1.0.0", &shellPaths{home: home, shell: "zsh"}, missingCommand); err != nil {
		t.Fatal(err)
	}
	if err := activate(root, source, "v1.0.0", &shellPaths{home: home, shell: "bash"}, missingCommand); err != nil {
		t.Fatal(err)
	}
	if err := remove(root, false, &shellPaths{home: home, shell: "bash"}, &removeVault{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".zshrc", ".zprofile", ".bashrc", ".profile"} {
		if _, err := os.Stat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("lost ownership of %s", name)
		}
	}
}
