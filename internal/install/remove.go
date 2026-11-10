package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mapherez/nox-cli/internal/credentials"
)

type remover interface {
	Removal(string, ownership) ([]pathEdit, error)
}

// Remove is called by a freshly verified script payload, not the installed binary.
func Remove(root string, purge bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return ErrInstall
	}
	return remove(root, purge, nativePaths(home), credentials.Native{})
}
func remove(root string, purge bool, paths pathManager, vault credentials.Vault) error {
	state, _, err := owned(root)
	if err != nil {
		binary := "nox"
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		_, stateErr := os.Lstat(filepath.Join(root, "install-state.json"))
		_, binaryErr := os.Lstat(filepath.Join(root, binary))
		if !errors.Is(stateErr, os.ErrNotExist) || !errors.Is(binaryErr, os.ErrNotExist) {
			return err
		}
		// A second uninstall can explicitly purge data retained by the first one.
		if purge {
			path := filepath.Join(root, "config.json")
			if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
				return ErrInstall
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return ErrInstall
			}
			if err := vault.Delete(); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return ErrInstall
			}
		}
		return nil
	}
	if _, err := os.Lstat(filepath.Join(root, "install", "update.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("An interrupted update must be recovered before uninstalling. Re-run the installer first.")
	}
	provider, ok := paths.(remover)
	if !ok {
		return ErrInstall
	}
	edits, err := provider.Removal(root, state)
	if err != nil {
		return err
	}
	configPath := filepath.Join(root, "config.json")
	if purge {
		if info, err := os.Lstat(configPath); err == nil && !info.Mode().IsRegular() {
			return ErrInstall
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrInstall
		}
		if err := vault.Delete(); err != nil {
			return err
		}
	}
	for i, e := range edits {
		if err := paths.Apply(e, false); err != nil {
			for j := i - 1; j >= 0; j-- {
				_ = paths.Apply(edits[j], true)
			}
			return err
		}
	}
	if os.Remove(filepath.Join(root, state.Binary)) != nil {
		for i := len(edits) - 1; i >= 0; i-- {
			_ = paths.Apply(edits[i], true)
		}
		return ErrInstall
	}
	if err := os.Remove(filepath.Join(root, "install-state.json")); err != nil {
		return ErrInstall
	}
	for _, hook := range state.Hooks {
		if hook.Created && hook.Added && filepath.IsAbs(hook.Target) {
			for _, edit := range edits {
				if hook.Target == edit.Target && len(edit.After) == 0 {
					if raw, err := readRegular(hook.Target); err == nil && len(raw) == 0 {
						_ = os.Remove(hook.Target)
					}
				}
			}
		}
	}
	if purge {
		if err := os.Remove(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrInstall
		}
	}
	// Delete only positively identified maintenance records, never arbitrary files.
	for _, name := range []string{"update-check.json", "update-result.json"} {
		path := filepath.Join(root, name)
		raw, err := readRegular(path)
		if err != nil {
			continue
		}
		var record struct {
			Product string `json:"product"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Product == "nox-cli" {
			_ = os.Remove(path)
		}
	}
	return nil
}
