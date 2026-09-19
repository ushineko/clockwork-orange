package platform

import (
	"bytes"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// packagedDaemon is the daemon as the packages install it, and the last resort
// when the running binary says nothing useful about where its sibling is.
const packagedDaemon = "/usr/bin/clockwork-orange"

// daemonName is the daemon's basename; guiSuffix is what the window's binary
// adds to it.
const (
	daemonName = "clockwork-orange"
	guiSuffix  = "-gui"
)

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

/*
installedUnit is the unit Install writes: for the daemon, with symlinks
resolved so the unit survives a re-install that replaces the link.

For the daemon, not for the running binary. The window can install the service
too, and os.Executable() there is clockwork-orange-gui -- which does not take
--service, exits 2 on the unknown flag, and with Restart=always is started
again ten seconds later for as long as the unit is enabled. What the user sees
is a service that will not come up and a Service section redrawing itself on
every status poll. See DaemonExecutable.
*/
func installedUnit() []byte {
	return UnitFileFor(DaemonExecutable())
}

/*
DaemonExecutable is the binary a unit should run: this one when the daemon is
running, and the daemon beside it when the window is.

The window's binary is the daemon's name plus "-gui", and the two are installed
together -- by the packages into /usr/bin, by install.sh into ~/.local/bin -- so
the sibling is the first place to look. Then $PATH, for a layout that separates
them. Then the packaged path, which is where a package put it even if this
process cannot see it.
*/
func DaemonExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return packagedDaemon
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return daemonFor(exe)
}

// daemonFor is DaemonExecutable's answer for a given running binary, so the
// fallbacks can be tested against a directory a test owns rather than against
// whatever the test binary happens to sit beside.
func daemonFor(exe string) string {
	sibling, isGUI := daemonBeside(exe)
	if !isGUI {
		return exe // the daemon is running: it is its own answer
	}
	if info, err := os.Stat(sibling); err == nil && !info.IsDir() {
		return sibling
	}
	if found, err := exec.LookPath(daemonName); err == nil {
		return found
	}
	return packagedDaemon
}

/*
daemonBeside is the daemon's path beside exe, and whether exe is the window at
all.

Split out because it is the part worth testing: it is pure, and the cases that
matter are a Windows ".exe", a path with "-gui" somewhere in a parent directory
rather than in the basename, and the daemon itself, which must be returned
untouched.
*/
func daemonBeside(exe string) (string, bool) {
	dir, base := filepath.Split(exe)
	ext := ""
	if dot := strings.LastIndex(base, "."); dot >= 0 && strings.EqualFold(base[dot:], ".exe") {
		ext, base = base[dot:], base[:dot]
	}
	if !strings.HasSuffix(base, guiSuffix) {
		return "", false
	}
	return filepath.Join(dir, strings.TrimSuffix(base, guiSuffix)+ext), true
}
