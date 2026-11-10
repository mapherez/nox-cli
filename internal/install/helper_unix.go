//go:build !windows

package install

import "os/exec"

func startHelper(*exec.Cmd) (int, error) { return 0, ErrInstall }
