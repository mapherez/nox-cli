package install

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The test executable is a disposable native helper; it never opens a vault or the user PATH.
func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "--complete-update" {
		source, _ := os.Executable()
		root := filepath.Dir(filepath.Dir(source))
		if FinishUpdate(context.Background(), root, source, os.Args[2], "v1.1.0") != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "--test-update-handoff" {
		root := os.Args[2]
		if HandoffWindows(root, filepath.Join(root, "install", "nox.exe")) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestWindowsHelperReplacesAnExecutableAfterItsProcessExits(t *testing.T) {
	root, source := staged(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, payload, 0700); err != nil {
		t.Fatal(err)
	}
	if err := activate(root, source, "v1.0.0", &testPaths{}, missingCommand); err != nil {
		t.Fatal(err)
	}
	// Distinct bytes, still a valid native PE test executable.
	if err := os.WriteFile(source, append(payload, []byte("candidate-v1.1.0")...), 0700); err != nil {
		t.Fatal(err)
	}
	parent := exec.Command(filepath.Join(root, "nox.exe"), "--test-update-handoff", root)
	if out, err := parent.CombinedOutput(); err != nil {
		t.Fatalf("helper launch: %v %s", err, out)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ := os.ReadFile(filepath.Join(root, "install-state.json"))
		var state ownership
		if json.Unmarshal(raw, &state) == nil && state.Version == "v1.1.0" {
			if _, err := os.Stat(filepath.Join(root, "install")); os.IsNotExist(err) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Native Windows helper did not complete update and cleanup")
}
