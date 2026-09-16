package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// resolvePath is Python's Path.resolve(): absolute, with symlinks followed
// when the target exists. A path that cannot be resolved is returned
// absolute so the caller's existence check produces the right message.
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// errNotExist is the "File does not exist" failure every setter reports.
func errNotExist(path string) error {
	return fmt.Errorf("file does not exist: %s", path)
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

// configHome is $XDG_CONFIG_HOME or ~/.config; it is where kwriteconfig6
// writes kscreenlockerrc and where the user systemd unit lives.
func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config")
}
