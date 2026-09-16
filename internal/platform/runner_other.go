//go:build !windows

package platform

import "os/exec"

// hideWindow is a no-op outside Windows; see runner_windows.go.
func hideWindow(*exec.Cmd) {}
