package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ushineko/clockwork-orange/internal/buildinfo"
)

/*
Version is the string the About section and `version` print (R7.12).

Order: the ldflags-stamped buildinfo.Version; when that is the "dev" default,
`.tag` next to the binary or in the working directory; and when a `.git`
directory is present beside `.tag`, `-r<count>-<short>` is appended the way
get_version_string did. "Unknown" when nothing applies.
*/
func Version() string {
	if buildinfo.Version != "" && buildinfo.Version != "dev" {
		return "v" + strings.TrimPrefix(buildinfo.Version, "v")
	}
	for _, dir := range candidateDirs() {
		b, err := os.ReadFile(filepath.Join(dir, ".tag"))
		if err != nil {
			continue
		}
		tag := strings.TrimSpace(string(b))
		if tag == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			if suffix := gitSuffix(dir); suffix != "" {
				return tag + suffix
			}
		}
		return tag
	}
	return "Unknown"
}

func candidateDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	return dirs
}

func gitSuffix(dir string) string {
	// dir is the binary's own directory or the cwd, not user input; the
	// arguments are fixed git verbs.
	count, err := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-list", "--count", "HEAD").Output() //nolint:gosec // fixed argv
	if err != nil {
		return ""
	}
	short, err := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--short", "HEAD").Output() //nolint:gosec // fixed argv
	if err != nil {
		return ""
	}
	return "-r" + strings.TrimSpace(string(count)) + "-" + strings.TrimSpace(string(short))
}
