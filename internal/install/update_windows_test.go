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

// Exercise the real shell through the production launcher. A successful
// CreateProcess alone does not establish that PowerShell executed the script.
func TestWindowsHelperExecutesPowerShellScript(t *testing.T) {
	root := filepath.Join(t.TempDir(), "helper with spaces")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "probe.ps1")
	marker := filepath.Join(root, "executed.txt")
	if err := os.WriteFile(script, []byte("param([string]$Marker)\n[IO.File]::WriteAllText($Marker, 'executed')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Marker", marker)
	pid, err := startHelper(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if processAlive(pid) {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(marker); err == nil && string(raw) == "executed" {
			return
		}
		if !processAlive(pid) {
			// The script may have written the marker and exited between checks.
			if raw, err := os.ReadFile(marker); err == nil && string(raw) == "executed" {
				return
			}
			t.Fatal("PowerShell exited before executing the helper script")
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("PowerShell did not execute the helper script")
}
