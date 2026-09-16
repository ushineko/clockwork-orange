package platform

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
)

// ServiceName is the systemd user unit the daemon runs as (R4.5).
const ServiceName = "clockwork-orange.service"

// UnitFile is the systemd unit as the packages ship it (R4.5): ExecStart names
// /usr/bin/clockwork-orange. It is embedded so the installed binary needs no
// source checkout, unlike the Python which copied the unit from the repository.
//
//go:embed clockwork-orange.service
var UnitFile []byte

// packagedExecStart is the line the packaged unit carries; Install rewrites it
// for a binary living anywhere else.
const packagedExecStart = "ExecStart=/usr/bin/clockwork-orange --service"

/*
UnitFileFor is the unit with ExecStart pointing at exe. A binary installed by
install.sh lives in ~/.local/bin, and a unit naming /usr/bin would fail to
start on the machine that installed it; the Python's service_install took
the repository path for the same reason. For /usr/bin/clockwork-orange the
result is UnitFile byte for byte.
*/
func UnitFileFor(exe string) []byte {
	return bytes.Replace(UnitFile, []byte(packagedExecStart), []byte("ExecStart="+exe+" --service"), 1)
}

// installedUnit is the unit Install writes: for the running binary, with
// symlinks resolved so the unit survives a re-install that replaces the link.
func installedUnit() []byte {
	exe, err := os.Executable()
	if err != nil {
		return UnitFile
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return UnitFileFor(exe)
}
