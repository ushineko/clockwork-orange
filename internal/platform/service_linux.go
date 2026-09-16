//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ushineko/clockwork-orange/internal/events"
)

const (
	systemctlTimeout  = 5 * time.Second
	journalctlTimeout = 10 * time.Second
)

// LinuxService controls the clockwork-orange.service user unit (R4.5).
type LinuxService struct {
	Runner Runner
	Events events.Events
}

// NewService returns the systemd implementation for this host.
func NewService(r Runner) Service { return &LinuxService{Runner: r} }

// NewServiceWithEvents is NewService with a log sink.
func NewServiceWithEvents(r Runner, ev events.Events) Service {
	return &LinuxService{Runner: r, Events: ev}
}

// Name implements Service.
func (*LinuxService) Name() string { return ServiceName }

// unitPath is $XDG_CONFIG_HOME/systemd/user/clockwork-orange.service.
func unitPath() string {
	return filepath.Join(configHome(), "systemd", "user", ServiceName)
}

func (s *LinuxService) systemctl(ctx context.Context, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, systemctlTimeout)
	defer cancel()
	argv := append([]string{"--user"}, args...)
	return s.Runner.Run(ctx, "systemctl", argv...)
}

// IsActive implements Service: `systemctl --user is-active`, whose trimmed
// stdout is the state even when the exit code is non-zero (inactive exits
// 3). No output at all -- missing systemctl, timeout -- is "unknown".
func (s *LinuxService) IsActive(ctx context.Context) ServiceState {
	stdout, _, _ := s.systemctl(ctx, "is-active", ServiceName)
	state := strings.TrimSpace(stdout)
	if state == "" {
		return StateUnknown
	}
	return ServiceState(state)
}

// StatusDetails implements Service: the text of `systemctl status`, or the
// error text when nothing was printed.
func (s *LinuxService) StatusDetails(ctx context.Context) string {
	stdout, stderr, err := s.systemctl(ctx, "status", ServiceName, "--no-pager")
	if stdout != "" || err == nil {
		return stdout
	}
	if stderr = strings.TrimSpace(stderr); stderr != "" {
		return err.Error() + "\n" + stderr
	}
	return err.Error()
}

func (s *LinuxService) checked(ctx context.Context, verb string) error {
	_, stderr, err := s.systemctl(ctx, verb, ServiceName)
	if err != nil {
		return fmt.Errorf("systemctl --user %s %s: %w: %s", verb, ServiceName, err, strings.TrimSpace(stderr))
	}
	return nil
}

// Start implements Service.
func (s *LinuxService) Start(ctx context.Context) error { return s.checked(ctx, "start") }

// Stop implements Service.
func (s *LinuxService) Stop(ctx context.Context) error { return s.checked(ctx, "stop") }

// Restart implements Service.
func (s *LinuxService) Restart(ctx context.Context) error { return s.checked(ctx, "restart") }

// Install implements Service: write the embedded unit with ExecStart naming
// this binary (UnitFileFor), daemon-reload, enable.
func (s *LinuxService) Install(ctx context.Context) error {
	path := unitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}
	// Unit files are conventionally world-readable; systemd --user only needs
	// the owner bit, but 0644 matches what shutil.copy2 produced before.
	if err := os.WriteFile(path, installedUnit(), 0o644); err != nil { //nolint:gosec // G306: see above.
		return fmt.Errorf("write unit file: %w", err)
	}
	s.Events.Debugf("Installed %s", path)
	if _, stderr, err := s.systemctl(ctx, "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %w: %s", err, strings.TrimSpace(stderr))
	}
	return s.checked(ctx, "enable")
}

// Uninstall implements Service: stop and disable (failures ignored), remove
// the unit file if present, daemon-reload.
func (s *LinuxService) Uninstall(ctx context.Context) error {
	_, _, _ = s.systemctl(ctx, "stop", ServiceName)
	_, _, _ = s.systemctl(ctx, "disable", ServiceName)
	path := unitPath()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove unit file: %w", err)
	}
	if _, stderr, err := s.systemctl(ctx, "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %w: %s", err, strings.TrimSpace(stderr))
	}
	return nil
}

// Logs implements Service: the last n journal lines for the unit. As in the
// Python, journalctl's stdout is returned whatever its exit code; only a
// failure to run it at all becomes the "Error retrieving logs" text.
func (s *LinuxService) Logs(ctx context.Context, lines int) string {
	ctx, cancel := context.WithTimeout(ctx, journalctlTimeout)
	defer cancel()
	stdout, _, err := s.Runner.Run(ctx, "journalctl", "--user", "-u", ServiceName, "--no-pager", "-n", strconv.Itoa(lines))
	if err != nil && stdout == "" {
		return fmt.Sprintf("Error retrieving logs: %v", err)
	}
	return stdout
}
