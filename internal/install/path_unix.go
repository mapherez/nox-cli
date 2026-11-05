//go:build !windows

package install

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const pathStart = "# >>> nox-cli installer >>>"
const pathEnd = "# <<< nox-cli installer <<<"

type shellPaths struct{ home, shell, stage string }

func nativePaths(home string) pathManager {
	return &shellPaths{home: home, shell: filepath.Base(os.Getenv("SHELL"))}
}

func (p *shellPaths) Prepare(root string) ([]pathEdit, error) {
	p.stage = filepath.Join(root, "install")
	if strings.Contains(root, ":") {
		return nil, errors.New("The application directory cannot contain the PATH separator.")
	}
	var targets []string
	switch p.shell {
	case "zsh":
		targets = []string{filepath.Join(p.home, ".zshrc"), filepath.Join(p.home, ".zprofile")}
	case "bash":
		targets = []string{filepath.Join(p.home, ".bashrc")}
		login := filepath.Join(p.home, ".profile")
		for _, name := range []string{".bash_profile", ".bash_login"} {
			if _, err := os.Lstat(filepath.Join(p.home, name)); err == nil {
				login = filepath.Join(p.home, name)
				break
			}
		}
		targets = append(targets, login)
	default:
		return nil, errors.New("The installer supports Bash and Zsh on this platform. No shell configuration was changed.")
	}
	quoted := "'" + strings.ReplaceAll(root, "'", "'\\''") + "'"
	block := pathStart + "\nexport PATH=" + quoted + ":\"$PATH\"\n" + pathEnd
	var edits []pathEdit
	for _, target := range targets {
		original, err := readRegular(target)
		exists := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, ErrInstall
		}
		if len(original) > 1<<20 {
			return nil, ErrInstall
		}
		mode := os.FileMode(0600)
		if exists {
			info, _ := os.Stat(target)
			mode = info.Mode().Perm()
		}
		text := string(original)
		if strings.Contains(text, pathStart) || strings.Contains(text, pathEnd) {
			if strings.Count(text, pathStart) != 1 || strings.Count(text, pathEnd) != 1 || !strings.Contains(text, block) {
				return nil, errors.New("An existing NoX CLI PATH block has changed. No shell configuration was replaced.")
			}
			edits = append(edits, pathEdit{Target: target, Before: original, After: original, Existed: exists, Mode: mode})
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += block + "\n"
		edits = append(edits, pathEdit{Target: target, Before: original, After: []byte(text), Existed: exists, Mode: mode})
	}
	return edits, nil
}

func (p *shellPaths) Apply(edit pathEdit, reverse bool) error {
	current, err := readRegular(edit.Target)
	expected, data := edit.Before, edit.After
	if reverse {
		expected, data = edit.After, edit.Before
	}
	if err != nil && !(errors.Is(err, os.ErrNotExist) && !edit.Existed && !reverse) {
		return ErrInstall
	}
	if !bytes.Equal(current, expected) {
		return ErrInstall
	}
	if reverse && !edit.Existed {
		return os.Remove(edit.Target)
	}
	if bytes.Equal(current, data) {
		return nil
	}
	return writeStaged(p.stage, edit.Target, data, edit.Mode)
}
