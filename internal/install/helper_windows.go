package install

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func startHelper(cmd *exec.Cmd) (int, error) {
	// PowerShell needs a console context even when non-interactive.
	// DETACHED_PROCESS can make it exit before executing its script; use the
	// windowless launch mode instead, independently of the CLI console.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
	if err := cmd.Start(); err != nil {
		return 0, ErrInstall
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
