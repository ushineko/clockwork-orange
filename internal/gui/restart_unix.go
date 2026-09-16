//go:build unix

package gui

import (
	"os"
	"syscall"
)

/*
restart replaces this process with a fresh one on the same arguments, so a
setting that Fyne fixes at window creation (the interface scale) takes
effect without the user finding the launcher again. The pending save is
written and the single-instance lock released first; exec(2) keeps the pid,
so a tray or launcher watching it sees one program.
*/
func (u *ui) restart() {
	u.shutdown()
	exe, err := os.Executable()
	if err != nil {
		u.app.Quit()
		return
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil { //nolint:gosec // our own binary, our own argv
		u.flash("Could not restart: "+err.Error()+". Close and reopen the window.", StatusWarn)
	}
}
