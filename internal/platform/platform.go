/*
Package platform is the port of platform_utils.py (spec 010 R4): the desktop
wallpaper and lock-screen setters, the systemd service controls, and the
single-instance lock, one implementation per OS behind build tags.

Everything that shells out goes through Runner, so tests substitute FakeRunner
and assert the exact argv that would have reached qdbus6, kwriteconfig6,
systemctl, journalctl, du or osascript. ExecRunner is the production Runner.

Logging: the Python module printed "[DEBUG]"/"[ERROR]" lines to stdout. Here
each concrete Platform and Service carries an exported Events field
(events.Events); New and NewService leave it zero (silent), NewWithEvents and
NewServiceWithEvents set it. Callers that hold only the interface use the
*WithEvents constructors; there is no setter on the interfaces so the
contract in docs/architecture.md is unchanged.
*/
package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"
)

// Monitor is one display's geometry in virtual-desktop coordinates (R4.6).
type Monitor struct{ X, Y, W, H int }

// ErrUnsupported is returned by operations the host OS has no equivalent for,
// notably SetLockscreen on Windows and macOS (R4.4).
var ErrUnsupported = errors.New("not supported on this platform")

// Runner runs an external program and captures its output. It is the single
// seam between this package and the operating system's tools.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

// Platform is the host desktop (R4.1-R4.4, R4.6-R4.9).
type Platform interface {
	// Name is "linux", "windows" or "darwin".
	Name() string
	// MonitorCount is the number of desktops/screens; any failure yields 1.
	MonitorCount(ctx context.Context) int
	SetWallpaper(ctx context.Context, path string) error
	SetWallpaperMulti(ctx context.Context, paths []string) error
	// SetLockscreen returns ErrUnsupported on windows and darwin.
	SetLockscreen(ctx context.Context, path string) error
	LockscreenSupported() bool
}

// ServiceState is what `systemctl is-active` prints (R4.5). The GUI switches
// on these exact strings.
type ServiceState string

// The states the service manager UI distinguishes.
const (
	StateActive       ServiceState = "active"
	StateInactive     ServiceState = "inactive"
	StateActivating   ServiceState = "activating"
	StateDeactivating ServiceState = "deactivating"
	StateFailed       ServiceState = "failed"
	StateUnknown      ServiceState = "unknown"
)

// Service controls the background service (R4.5). On non-Linux hosts every
// method is a no-op that returns the informational strings the Python
// implementation returned.
type Service interface {
	Name() string
	IsActive(ctx context.Context) ServiceState
	StatusDetails(ctx context.Context) string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Install(ctx context.Context) error
	Uninstall(ctx context.Context) error
	Logs(ctx context.Context, lines int) string
}

// Lock is a held single-instance lock (R4.8, R4.11, DV10).
type Lock interface{ Release() }

// ExecRunner runs programs with os/exec. On Windows the child gets
// CREATE_NO_WINDOW so a GUI process does not flash console windows.
type ExecRunner struct{}

// Run implements Runner. A non-zero exit returns *exec.ExitError; a missing
// program returns an error satisfying errors.Is(err, exec.ErrNotFound); a
// context deadline returns an error wrapping ctx.Err().
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: Runner is the audited subprocess boundary; callers build fixed argv.
	hideWindow(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out.String(), errb.String(), fmt.Errorf("%s: %w", name, ctxErr)
		}
		return out.String(), errb.String(), fmt.Errorf("%s: %w", name, err)
	}
	return out.String(), errb.String(), nil
}

// FakeRunner records every argv and answers from Handler, or from the fixed
// Stdout/Stderr/Err when Handler is nil. Safe for concurrent use.
type FakeRunner struct {
	mu    sync.Mutex
	calls [][]string

	// Handler, when set, decides the response per call. argv[0] is the
	// program name.
	Handler func(ctx context.Context, argv []string) (stdout, stderr string, err error)
	Stdout  string
	Stderr  string
	Err     error
}

// Run implements Runner.
func (f *FakeRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	argv := append([]string{name}, args...)
	f.mu.Lock()
	f.calls = append(f.calls, argv)
	f.mu.Unlock()
	if f.Handler != nil {
		return f.Handler(ctx, argv)
	}
	return f.Stdout, f.Stderr, f.Err
}

// Calls returns a copy of every argv recorded so far, oldest first.
func (f *FakeRunner) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = append([]string(nil), c...)
	}
	return out
}

// Reset forgets the recorded calls.
func (f *FakeRunner) Reset() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
}

// JSStringEscape makes s safe inside a double-quoted JavaScript string
// literal (DV1): backslash and double quote are backslash-escaped and control
// characters become \n, \r, \t or \u00XX. Paths without those characters are
// returned unchanged, which is what keeps the golden KDE scripts byte-equal.
func JSStringEscape(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r == '\\' || r == '"' || r < 0x20 }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
