package install

import (
	"os/exec"
	"syscall"
)

func startHelper(cmd *exec.Cmd) (int, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x00000008}
	if err := cmd.Start(); err != nil {
		return 0, ErrInstall
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
