package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/mapherez/nox-cli/internal/config"
)

var ErrManaged = errors.New("Updates require a script-installed NoX CLI. Source builds are not replaced.")

type updateJournal struct {
	Schema       int       `json:"schema"`
	Product      string    `json:"product"`
	Before       string    `json:"before"`
	After        string    `json:"after"`
	AfterVersion string    `json:"afterVersion,omitempty"`
	State        ownership `json:"state"`
}

type pendingUpdate struct {
	Product string `json:"product"`
	Parent  int    `json:"parentPid"`
	Helper  int    `json:"helperPid,omitempty"`
}

func MarkPending(root string) error {
	data, _ := json.Marshal(pendingUpdate{Product: "nox-cli", Parent: os.Getpid()})
	return writeStaged(filepath.Join(root, "install"), filepath.Join(root, "install", "update-pending.json"), data, 0600)
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func owned(root string) (ownership, []byte, error) {
	var state ownership
	for _, dir := range []string{root, filepath.Join(root, "install")} {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return state, nil, ErrManaged
		}
	}
	raw, err := readRegular(filepath.Join(root, "install-state.json"))
	binary := "nox"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err != nil || json.Unmarshal(raw, &state) != nil || state.Schema != 1 || state.Product != "nox-cli" || state.Binary != binary {
		return state, nil, ErrManaged
	}
	payload, err := readRegular(filepath.Join(root, binary))
	if err != nil || digest(payload) != state.SHA256 {
		return state, nil, ErrManaged
	}
	return state, payload, nil
}

// Managed verifies both ownership and the executing path. It never reads a vault.
func Managed(root string) bool {
	source, err := os.Executable()
	if err != nil {
		return false
	}
	binary := "nox"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if !samePath(source, filepath.Join(root, binary)) {
		return false
	}
	// Initial installs remove staging when idle.
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	raw, err := readRegular(filepath.Join(root, "install-state.json"))
	var state ownership
	payload, readErr := readRegular(source)
	return err == nil && readErr == nil && json.Unmarshal(raw, &state) == nil && state.Schema == 1 && state.Product == "nox-cli" && state.Binary == binary && digest(payload) == state.SHA256
}

// Stage holds the same lock as the script installer for a bounded update.
func Stage(root string) (string, func(bool), error) {
	if err := (config.Store{Root: root}).Directory(); err != nil {
		return "", nil, err
	}
	stage := filepath.Join(root, "install")
	if err := os.Mkdir(stage, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", nil, ErrInstall
	}
	info, err := os.Lstat(stage)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, ErrInstall
	}
	unlock, err := lockStage(stage)
	if err != nil {
		return "", nil, err
	}
	if raw, err := readRegular(filepath.Join(stage, "update-pending.json")); err == nil {
		var pending pendingUpdate
		if json.Unmarshal(raw, &pending) != nil || pending.Product != "nox-cli" || pending.Parent <= 0 || processAlive(pending.Parent) || (pending.Helper > 0 && processAlive(pending.Helper)) {
			unlock()
			return "", nil, ErrInstall
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		unlock()
		return "", nil, ErrInstall
	}
	if err := recoverUpdate(root, stage); err != nil {
		unlock()
		return "", nil, err
	}
	for _, name := range []string{"nox", "nox.exe", "update-helper.ps1"} {
		path := filepath.Join(stage, name)
		if info, err := os.Lstat(path); err == nil {
			if !info.Mode().IsRegular() {
				unlock()
				return "", nil, ErrInstall
			}
			if os.Remove(path) != nil {
				unlock()
				return "", nil, ErrInstall
			}
		}
	}
	return stage, func(keep bool) {
		if !keep {
			cleanStage(stage)
		}
		unlock()
		if !keep {
			_ = os.Remove(stage)
		}
	}, nil
}
func cleanStage(stage string) {
	// A journal is retained only if recovery failed; never discard its backups.
	if _, err := os.Lstat(filepath.Join(stage, "update.json")); err == nil {
		return
	}
	for _, name := range []string{"nox", "nox.exe", "previous.bin", "previous-state.json", "update-helper.ps1", "update-pending.json"} {
		_ = os.Remove(filepath.Join(stage, name))
	}
	entries, _ := os.ReadDir(stage)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "activate-") && entry.Type().IsRegular() {
			_ = os.Remove(filepath.Join(stage, entry.Name()))
		}
	}
}

// Apply replaces only the installer-owned executable and preserves its PATH hooks.
func Apply(ctx context.Context, root, source, version string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	stage := filepath.Join(root, "install")
	state, previous, err := owned(root)
	if err != nil {
		return err
	}
	if !samePath(source, filepath.Join(stage, state.Binary)) {
		return ErrInstall
	}
	payload, err := readRegular(source)
	if err != nil {
		return ErrInstall
	}
	raw, _ := readRegular(filepath.Join(root, "install-state.json"))
	journal := updateJournal{Schema: 1, Product: "nox-cli", Before: digest(previous), After: digest(payload), AfterVersion: version, State: state}
	data, _ := json.Marshal(journal)
	for _, file := range []struct {
		name string
		data []byte
	}{{"previous.bin", previous}, {"previous-state.json", raw}, {"update.json", data}} {
		if err := writeStaged(stage, filepath.Join(stage, file.name), file.data, 0600); err != nil {
			return err
		}
	}
	rollback := func(err error) error {
		if recoverErr := recoverUpdate(root, stage); recoverErr != nil {
			return errors.New("Update recovery is required. Re-run the installer; your settings are preserved.")
		}
		return err
	}
	if ctx.Err() != nil {
		return rollback(ctx.Err())
	}
	if err := writeStaged(stage, filepath.Join(root, state.Binary), payload, 0700); err != nil {
		return rollback(err)
	}
	state.SHA256 = digest(payload)
	state.Version = version
	updated, _ := json.MarshalIndent(state, "", "  ")
	if err := writeStaged(stage, filepath.Join(root, "install-state.json"), append(updated, '\n'), 0600); err != nil {
		return rollback(err)
	}
	if err := os.Remove(filepath.Join(stage, "update.json")); err != nil {
		return rollback(ErrInstall)
	}
	_ = os.Remove(filepath.Join(stage, "previous.bin"))
	_ = os.Remove(filepath.Join(stage, "previous-state.json"))
	return nil
}
func recoverUpdate(root, stage string) error {
	raw, err := readRegular(filepath.Join(stage, "update.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return ErrInstall
	}
	var journal updateJournal
	if json.Unmarshal(raw, &journal) != nil || journal.Schema != 1 || journal.Product != "nox-cli" {
		return ErrInstall
	}
	name := "nox"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if journal.State.Binary != name || journal.State.Schema != 1 || journal.State.Product != "nox-cli" || journal.State.SHA256 != journal.Before {
		return ErrInstall
	}
	before, err := readRegular(filepath.Join(stage, "previous.bin"))
	if err != nil || digest(before) != journal.Before {
		return ErrInstall
	}
	stateBytes, err := readRegular(filepath.Join(stage, "previous-state.json"))
	var prior ownership
	if err != nil || json.Unmarshal(stateBytes, &prior) != nil || prior.SHA256 != journal.Before || prior.Binary != name || prior.Product != "nox-cli" || prior.Schema != 1 {
		return ErrInstall
	}
	current, err := readRegular(filepath.Join(root, name))
	if err != nil || (digest(current) != journal.Before && digest(current) != journal.After) {
		return ErrInstall
	}
	source, _ := os.Executable()
	if runtime.GOOS == "windows" && samePath(source, filepath.Join(root, name)) && digest(current) == journal.After {
		if journal.AfterVersion == "" {
			return ErrInstall
		}
		// A restarted Windows image cannot replace itself; complete the verified commit.
		prior.SHA256 = journal.After
		prior.Version = journal.AfterVersion
		stateBytes, _ = json.MarshalIndent(prior, "", "  ")
	} else if digest(current) != journal.Before {
		if writeStaged(stage, filepath.Join(root, name), before, 0700) != nil {
			return ErrInstall
		}
	}
	if writeStaged(stage, filepath.Join(root, "install-state.json"), stateBytes, 0600) != nil {
		return ErrInstall
	}
	if os.Remove(filepath.Join(stage, "update.json")) != nil {
		return ErrInstall
	}
	_ = os.Remove(filepath.Join(stage, "previous.bin"))
	_ = os.Remove(filepath.Join(stage, "previous-state.json"))
	return nil
}

// Recover cleans only an interrupted owned update, without touching credentials.
func Recover(root string) error {
	stage := filepath.Join(root, "install")
	_, journalErr := os.Lstat(filepath.Join(stage, "update.json"))
	_, pendingErr := os.Lstat(filepath.Join(stage, "update-pending.json"))
	if errors.Is(journalErr, os.ErrNotExist) && errors.Is(pendingErr, os.ErrNotExist) {
		return nil
	}
	_, done, err := Stage(root)
	if err != nil {
		return err
	}
	done(false)
	return nil
}

// FinishUpdate is the verified Windows payload handoff after the old process exits.
func FinishUpdate(ctx context.Context, root, source, expected, version string) error {
	stage := filepath.Join(root, "install")
	unlock, err := lockStage(stage)
	if err != nil {
		return err
	}
	defer unlock()
	state, _, err := owned(root)
	if err != nil || state.SHA256 != expected {
		return ErrManaged
	}
	err = Apply(ctx, root, source, version)
	result := struct {
		Product string `json:"product"`
		Message string `json:"message"`
	}{"nox-cli", "NoX CLI updated. Restart nox to use the new version."}
	if err != nil {
		result.Message = "The update did not complete. Re-run the installer to recover; your settings were not changed."
	}
	raw, _ := json.Marshal(result)
	_ = config.WriteFile(filepath.Join(root, "update-result.json"), append(raw, '\n'), 0600)
	return err
}
func HandoffWindows(root, source string) error {
	stage := filepath.Join(root, "install")
	state, _, err := owned(root)
	if err != nil {
		return err
	}
	if !samePath(source, filepath.Join(stage, state.Binary)) {
		return ErrInstall
	}
	helper := filepath.Join(stage, "update-helper.ps1")
	const script = `param([int]$ParentId, [string]$Expected)
$ErrorActionPreference = 'Stop'
$stage = $PSScriptRoot
try {
    Wait-Process -Id $ParentId -Timeout 90 -ErrorAction SilentlyContinue
    if (Get-Process -Id $ParentId -ErrorAction SilentlyContinue) { throw 'CLI is still running.' }
    & (Join-Path $stage 'nox.exe') --complete-update $Expected
    if ($LASTEXITCODE -ne 0) { throw 'Native update did not complete.' }
} finally {
    foreach ($name in @('nox.exe','update-helper.ps1','update-pending.json')) {
        $path = Join-Path $stage $name
        for ($attempt = 0; $attempt -lt 30 -and (Test-Path -LiteralPath $path); $attempt++) {
            Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue
            if (Test-Path -LiteralPath $path) { Start-Sleep -Milliseconds 100 }
        }
    }
    if ([IO.Directory]::Exists($stage) -and [IO.Directory]::GetFileSystemEntries($stage).Length -eq 0) { [IO.Directory]::Delete($stage) }
}
`
	if err := os.WriteFile(helper, []byte(script), 0600); err != nil {
		return ErrInstall
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", helper, "-ParentId", strconv.Itoa(os.Getpid()), "-Expected", state.SHA256)
	pid, err := startHelper(cmd)
	if err != nil {
		return err
	}
	pending, _ := json.Marshal(pendingUpdate{Product: "nox-cli", Parent: os.Getpid(), Helper: pid})
	if err := writeStaged(stage, filepath.Join(stage, "update-pending.json"), pending, 0600); err != nil {
		if process, findErr := os.FindProcess(pid); findErr == nil {
			_ = process.Kill()
		}
		return ErrInstall
	}
	return nil
}
