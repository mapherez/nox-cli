// Package install activates only the verified executable handed off by an installer.
package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

var ErrFilesystem = errors.New("The application directory and shell profiles must share a filesystem for atomic installation.")

var ErrInstall = errors.New("Installation could not be completed. Close any running NoX CLI and re-run the installer.")

type pathEdit struct {
	Target        string
	Before, After []byte
	Existed       bool
	Mode          os.FileMode
	ValueType     uint32
}
type pathManager interface {
	Prepare(string) ([]pathEdit, error)
	Apply(pathEdit, bool) error
}
type hookRecord struct {
	Target string `json:"target"`
	Added  bool   `json:"added"`
}
type ownership struct {
	Schema  int          `json:"schema"`
	Product string       `json:"product"`
	Binary  string       `json:"binary"`
	SHA256  string       `json:"sha256"`
	Version string       `json:"version"`
	Hooks   []hookRecord `json:"pathHooks"`
}

// Complete is an internal installer handoff, not a manual installation method.
func Complete(root, source, version string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return ErrInstall
	}
	return activate(root, source, version, nativePaths(home), exec.LookPath)
}

func activate(root, source, version string, paths pathManager, lookPath func(string) (string, error)) error {
	binary := "nox"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	stage := filepath.Join(root, "install")
	sourceAbs, err := filepath.Abs(source)
	if err != nil || !samePath(sourceAbs, filepath.Join(stage, binary)) {
		return ErrInstall
	}
	for _, path := range []string{root, stage} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrInstall
		}
	}
	info, err := os.Lstat(sourceAbs)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return ErrInstall
	}
	target := filepath.Join(root, binary)
	statePath := filepath.Join(root, "install-state.json")
	priorState, stateErr := readRegular(statePath)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return ErrInstall
	}
	priorBinary, binaryErr := readRegular(target)
	if binaryErr != nil && !errors.Is(binaryErr, os.ErrNotExist) {
		return ErrInstall
	}
	var previous ownership
	if stateErr == nil {
		if json.Unmarshal(priorState, &previous) != nil || previous.Schema != 1 || previous.Product != "nox-cli" || previous.Binary != binary {
			return ErrInstall
		}
	} else if binaryErr == nil {
		return errors.New("The target executable is not owned by the NoX CLI installer.")
	}
	if existing, err := lookPath("nox"); err == nil {
		existing, _ = filepath.Abs(existing)
		if !samePath(existing, target) {
			return errors.New("A different nox command already exists on PATH. No files were replaced.")
		}
	}
	edits, err := paths.Prepare(root)
	if err != nil {
		return err
	}
	payload, err := readRegular(sourceAbs)
	if err != nil {
		return ErrInstall
	}
	digest := sha256.Sum256(payload)
	state := ownership{Schema: 1, Product: "nox-cli", Binary: binary, SHA256: hex.EncodeToString(digest[:]), Version: version, Hooks: []hookRecord{}}
	for _, edit := range edits {
		added := !bytes.Equal(edit.Before, edit.After)
		for _, old := range previous.Hooks {
			if old.Target == edit.Target && old.Added {
				added = true
			}
		}
		state.Hooks = append(state.Hooks, hookRecord{Target: edit.Target, Added: added})
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return ErrInstall
	}
	data = append(data, '\n')
	// Write ownership before activation so a process interruption is recoverable by
	// the next verified installer, without treating an unknown binary as ours.
	if err := writeStaged(stage, statePath, data, 0600); err != nil {
		return err
	}
	restoreFiles := func() {
		if binaryErr == nil {
			_ = writeStaged(stage, target, priorBinary, 0700)
		} else {
			_ = os.Remove(target)
		}
		if stateErr == nil {
			_ = writeStaged(stage, statePath, priorState, 0600)
		} else {
			_ = os.Remove(statePath)
		}
	}
	if err := writeStaged(stage, target, payload, 0700); err != nil {
		restoreFiles()
		return ErrInstall
	}
	for i, edit := range edits {
		if err := paths.Apply(edit, false); err != nil {
			for j := i - 1; j >= 0; j-- {
				_ = paths.Apply(edits[j], true)
			}
			restoreFiles()
			return err
		}
	}
	return nil
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return nil, ErrInstall
	}
	return os.ReadFile(path)
}

func writeStaged(stage, target string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(target); err == nil && !info.Mode().IsRegular() {
		return ErrInstall
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrInstall
	}
	file, err := os.CreateTemp(stage, "activate-")
	if err != nil {
		return ErrInstall
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(mode); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil || closed != nil {
		return ErrInstall
	}
	if err := os.Rename(name, target); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return ErrFilesystem
		}
		return ErrInstall
	}
	return nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
