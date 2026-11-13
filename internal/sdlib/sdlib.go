/*
Package sdlib finds the stable-diffusion.cpp shared libraries the
stable_diffusion plugin loads (spec 031 R5).

The libraries are built in CI at the version malina pins (spec 031 R9) and
embedded in the binary behind the sdembed build tag. A default build embeds
nothing and CI is unaffected; such a build still works when
CLOCKWORK_SD_LIB names a directory that holds the libraries, which is how
development and the live tests run.

Loading a library that does not match the binding crashes the process rather
than returning an error (spec 030), so an embedded library carries a VERSION
file and Resolve refuses a mismatch before anything is loaded.
*/
package sdlib

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// EnvDir names a directory of stable-diffusion.cpp libraries that takes
// precedence over the embedded copy.
const EnvDir = "CLOCKWORK_SD_LIB"

// PinnedVersion is the stable-diffusion.cpp release the malina module in
// go.mod is built against (malina v1.2.2). Move it with the module.
const PinnedVersion = "master-1088-32d516c"

// versionFile is the name of the file in an embedded library directory that
// records the stable-diffusion.cpp release it was built from.
const versionFile = "VERSION"

// ErrNoLibrary reports a build that embeds no library when none is supplied.
var ErrNoLibrary = errors.New("this build has no Stable Diffusion library: " +
	"set " + EnvDir + " to a directory of stable-diffusion.cpp libraries, " +
	"or build with -tags sdembed")

// Resolve returns a directory that holds the native libraries: EnvDir when
// set, otherwise the embedded copy extracted into the user cache.
func Resolve() (string, error) {
	if dir := os.Getenv(EnvDir); dir != "" {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() { //nolint:gosec // the user's own override directory
			return "", fmt.Errorf("%s=%q is not a directory", EnvDir, dir)
		}
		return dir, nil
	}
	if embedded == nil {
		return "", ErrNoLibrary
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("find cache directory: %w", err)
	}
	return Extract(embedded, filepath.Join(cache, "clockwork-orange", "sd"), PinnedVersion)
}

// Embedded reports whether this build carries a library.
func Embedded() bool { return embedded != nil }

// Extract copies the files of src (rooted at ".") into
// <cacheRoot>/<content-hash>/ and returns that directory. It is idempotent:
// a directory that already exists is returned untouched. The copy is staged
// in a sibling directory and renamed into place, so a concurrent or
// interrupted extraction never leaves a half-written library to be loaded.
// want is the version the VERSION file must name.
func Extract(src fs.FS, cacheRoot, want string) (string, error) {
	got, err := fs.ReadFile(src, versionFile)
	if err != nil {
		return "", fmt.Errorf("embedded library has no %s file: %w", versionFile, err)
	}
	if string(trimNL(got)) != want {
		return "", fmt.Errorf("embedded library is %q but the binding needs %q",
			trimNL(got), want)
	}
	files, err := listFiles(src)
	if err != nil {
		return "", err
	}
	sum, err := fingerprint(src, files)
	if err != nil {
		return "", err
	}
	final := filepath.Join(cacheRoot, sum)
	if st, err := os.Stat(final); err == nil && st.IsDir() {
		return final, nil
	}
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", cacheRoot, err)
	}
	stage, err := os.MkdirTemp(cacheRoot, ".extract-")
	if err != nil {
		return "", fmt.Errorf("stage extraction: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for _, name := range files {
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return "", fmt.Errorf("read embedded %s: %w", name, err)
		}
		dst := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return "", fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
		}
		// Shared libraries are mapped, not executed, but the loader of some
		// platforms insists on the execute bit.
		if err := os.WriteFile(dst, data, 0o700); err != nil { //nolint:gosec // library files
			return "", fmt.Errorf("write %s: %w", dst, err)
		}
	}
	if err := os.Rename(stage, final); err != nil {
		// Another process won the race; its copy is as good as ours.
		if st, serr := os.Stat(final); serr == nil && st.IsDir() {
			return final, nil
		}
		return "", fmt.Errorf("install %s: %w", final, err)
	}
	return final, nil
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

// listFiles returns every regular file of src, slash-separated and sorted.
func listFiles(src fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && path.Base(p) != ".gitkeep" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list embedded library: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 || (len(files) == 1 && files[0] == versionFile) {
		return nil, ErrNoLibrary
	}
	return files, nil
}

// fingerprint names the extraction directory: a hash of the file names and
// contents, so a rebuilt library never reuses a stale directory.
func fingerprint(src fs.FS, files []string) (string, error) {
	h := sha256.New()
	for _, name := range files {
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return "", fmt.Errorf("read embedded %s: %w", name, err)
		}
		_, _ = fmt.Fprintf(h, "%s:%d;", name, len(data))
		_, _ = h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
