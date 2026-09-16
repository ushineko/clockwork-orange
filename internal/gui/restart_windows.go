//go:build windows

package gui

import (
	"os"
	"os/exec"
)

// restart starts a fresh copy of this program and quits; Windows has no
// exec(2), so the new process gets a new pid.
func (u *ui) restart() {
	u.shutdown()
	exe, err := os.Executable()
	if err == nil {
		cmd := exec.Command(exe, os.Args[1:]...) //nolint:gosec // our own binary, our own argv
		cmd.Env = os.Environ()
		_ = cmd.Start()
	}
	u.app.Quit()
}
