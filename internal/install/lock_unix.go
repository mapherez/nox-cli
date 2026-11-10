//go:build !windows

package install

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func lockStage(stage string) (func(), error) {
	lock := filepath.Join(stage, "installer.lock")
	if info, err := os.Lstat(lock); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrInstall
		}
		raw, err := readRegular(filepath.Join(lock, "pid"))
		if err != nil {
			return nil, ErrInstall
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil || pid <= 0 {
			return nil, ErrInstall
		}
		if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
			return nil, ErrInstall
		}
		if os.Remove(filepath.Join(lock, "pid")) != nil || os.Remove(lock) != nil {
			return nil, ErrInstall
		}
	}
	if os.Mkdir(lock, 0700) != nil {
		return nil, ErrInstall
	}
	if os.WriteFile(filepath.Join(lock, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0600) != nil {
		_ = os.Remove(lock)
		return nil, ErrInstall
	}
	return func() { _ = os.Remove(filepath.Join(lock, "pid")); _ = os.Remove(lock) }, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) != syscall.ESRCH
}
