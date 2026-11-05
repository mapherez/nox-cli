// Package config owns non-secret, per-user settings and installation paths.
package config

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const CredentialRef = "nox-cli/api-key"

var ErrSettings = errors.New("Unable to read or save CLI settings.")

type Settings struct {
	Version       int    `json:"version"`
	GatewayOrigin string `json:"gatewayOrigin"`
	CredentialRef string `json:"credentialRef,omitempty"`
	SessionOnly   bool   `json:"sessionOnly,omitempty"`
}

func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ErrSettings
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, err = os.UserCacheDir()
		}
		if err != nil || !filepath.IsAbs(base) {
			return "", ErrSettings
		}
		return filepath.Join(base, "nox-cli"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nox-cli"), nil
	case "linux":
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		if !filepath.IsAbs(base) {
			return "", ErrSettings
		}
		return filepath.Join(base, "nox-cli"), nil
	default:
		return "", errors.New("This operating system is not supported.")
	}
}

type Store struct{ Root string }

// Directory rejects links instead of following a replaced application directory.
func (s Store) Directory() error {
	if !filepath.IsAbs(s.Root) {
		return ErrSettings
	}
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return ErrSettings
	}
	info, err := os.Lstat(s.Root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrSettings
	}
	if runtime.GOOS != "windows" && os.Chmod(s.Root, 0700) != nil {
		return ErrSettings
	}
	return nil
}

func (s Store) Load() (Settings, error) {
	var result Settings
	path := filepath.Join(s.Root, "config.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return result, ErrSettings
	}
	file, err := os.Open(path)
	if err != nil {
		return result, ErrSettings
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return Settings{}, ErrSettings
	}
	if result.Version != 1 || result.GatewayOrigin == "" || (result.SessionOnly && result.CredentialRef != "") || (result.CredentialRef != "" && result.CredentialRef != CredentialRef) {
		return Settings{}, ErrSettings
	}
	return result, nil
}

func (s Store) Save(settings Settings) error {
	if err := s.Directory(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return ErrSettings
	}
	return WriteFile(filepath.Join(s.Root, "config.json"), append(data, '\n'), 0600)
}

// WriteFile stages beside the target and atomically replaces only a regular file.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return ErrSettings
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrSettings
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".nox-cli-write-")
	if err != nil {
		return ErrSettings
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(mode); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrSettings
	}
	if err := os.Rename(name, path); err != nil {
		return ErrSettings
	}
	return nil
}
