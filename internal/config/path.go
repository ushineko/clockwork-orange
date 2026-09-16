/*
Package config holds clockwork-orange's on-disk settings: the YAML document at
~/.config/clockwork-orange.yml, the state directory beside it, migrations, and
the file watcher the daemon uses (spec 010 R2).

Path helpers are copied from nmsbonker (same author) — keep in sync by hand.
The location rules are this project's own (R2.1): the file lives at the
literal ~/.config on every OS, because that is where every 2.9.x install put
it and the port must read those files unchanged.
*/
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FileName is the config file's basename.
const FileName = "clockwork-orange.yml"

// StateDirName is the directory beside the config file that holds history.db
// and blacklist.db.
const StateDirName = "clockwork-orange"

// windowsPublicPath is probed second on Windows (R2.1); it was written by the
// never-shipped Windows service and is read for compatibility only.
const windowsPublicPath = `C:/Users/Public/clockwork_config.yml`

// HomeDir returns the user's home directory, honouring $HOME (and
// %USERPROFILE% on Windows) so tests can redirect it.
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

// Dir is ~/.config on every OS (R2.1).
func Dir() string { return filepath.Join(HomeDir(), ".config") }

// DefaultPath is where writers always write: ~/.config/clockwork-orange.yml.
func DefaultPath() string { return filepath.Join(Dir(), FileName) }

// StateDir is ~/.config/clockwork-orange/, home of the SQLite stores.
func StateDir() string { return filepath.Join(Dir(), StateDirName) }

// CandidatePaths lists the paths probed on load, in order (R2.1).
func CandidatePaths() []string {
	paths := []string{DefaultPath()}
	if runtime.GOOS == "windows" {
		paths = append(paths, windowsPublicPath)
	}
	return paths
}

// ExpandPath resolves a leading "~".
//
// Only a leading one: a tilde in the middle of a path is an ordinary filename
// to every shell, and quietly rewriting it would move a directory the user
// actually named.
func ExpandPath(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home := HomeDir()
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// ErrTildeComponent reports a path with a component that is literally "~".
var ErrTildeComponent = errors.New("path component is a bare tilde")

// CheckCreatablePath refuses to create anything under a path component that is
// literally "~" (angou's rule).
//
// A directory named "~" is a trap: from inside its parent, the obvious way to
// remove it is `rm -rf ~`, which the shell expands to the user's home directory
// before rm ever runs. ExpandPath has already resolved a *leading* tilde, so
// anything reaching here is a tilde deeper in the path, where expansion does not
// apply and never will.
func CheckCreatablePath(p string) error {
	for _, part := range strings.Split(filepath.Clean(p), string(filepath.Separator)) {
		if part != "~" {
			continue
		}
		return fmt.Errorf("%w: %s\n"+
			"Refusing to create anything under a directory named \"~\". From its parent, the "+
			"obvious way to remove it is `rm -rf ~`, which the shell expands to your home "+
			"directory before rm runs.\n"+
			"If you meant your home directory, write it as ~/ at the start of the path, or "+
			"give the full path", ErrTildeComponent, p)
	}
	return nil
}

// MkdirAll creates a directory lazily, at the moment it is needed, and never
// under a bare-tilde component.
func MkdirAll(dir string) error {
	if err := CheckCreatablePath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return nil
}
