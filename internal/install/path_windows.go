package install

import (
	"bytes"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

type windowsPaths struct{ keyPath string }

func nativePaths(string) pathManager { return windowsPaths{} }
func (p windowsPaths) key() string {
	if p.keyPath != "" {
		return p.keyPath
	}
	return `Environment`
}

func (p windowsPaths) userPath() (string, uint32, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, p.key(), registry.QUERY_VALUE)
	if err != nil {
		return "", 0, ErrInstall
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, ErrInstall
	}
	return value, kind, nil
}

func (p windowsPaths) Prepare(root string) ([]pathEdit, error) {
	if strings.Contains(root, ";") {
		return nil, ErrInstall
	}
	before, kind, err := p.userPath()
	if err != nil {
		return nil, err
	}
	after := before
	found := false
	for _, part := range strings.Split(before, ";") {
		expanded, err := registry.ExpandString(strings.Trim(part, `"`))
		if err != nil {
			expanded = part
		}
		if strings.EqualFold(filepath.Clean(expanded), root) {
			found = true
		}
	}
	if !found {
		if after != "" && !strings.HasSuffix(after, ";") {
			after += ";"
		}
		after += root
	}
	return []pathEdit{{Target: "HKCU/Environment/Path", Before: []byte(before), After: []byte(after), Existed: kind != 0, Mode: 0600, ValueType: kind}}, nil
}

func (p windowsPaths) Apply(edit pathEdit, reverse bool) error {
	current, kind, err := p.userPath()
	if err != nil {
		return err
	}
	expected, data := edit.Before, edit.After
	if reverse {
		expected, data = edit.After, edit.Before
	}
	if !bytes.Equal([]byte(current), expected) {
		return ErrInstall
	}
	if bytes.Equal([]byte(current), data) {
		return nil
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, p.key(), registry.SET_VALUE)
	if err != nil {
		return ErrInstall
	}
	defer key.Close()
	if reverse && !edit.Existed {
		if key.DeleteValue("Path") != nil {
			return ErrInstall
		}
	} else {
		valueType := edit.ValueType
		if valueType == 0 {
			valueType = registry.EXPAND_SZ
		}
		if kind != 0 && !reverse && kind != valueType {
			return ErrInstall
		}
		if valueType == registry.SZ {
			err = key.SetStringValue("Path", string(data))
		} else {
			err = key.SetExpandStringValue("Path", string(data))
		}
		if err != nil {
			return ErrInstall
		}
	}
	if p.keyPath == "" {
		message, _ := syscall.UTF16PtrFromString("Environment")
		var result uintptr
		syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(0xffff, 0x1a, 0, uintptr(unsafe.Pointer(message)), 2, 1000, uintptr(unsafe.Pointer(&result)))
	}
	return nil
}
