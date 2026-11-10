package install

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func lockStage(stage string) (func(), error) {
	path := filepath.Join(stage, "installer.lock")
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, ErrInstall
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrInstall
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, ErrInstall
	}
	return func() { _ = windows.CloseHandle(handle); _ = os.Remove(path) }, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return err != windows.ERROR_INVALID_PARAMETER
	}
	defer windows.CloseHandle(handle)
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) != nil || code == 259
}
