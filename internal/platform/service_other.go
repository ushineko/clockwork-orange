//go:build !linux

package platform

import (
	"context"
	"runtime"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// NoService is the non-Linux Service (R4.5): there is no background service
// on Windows or macOS (the tray app cycles instead), so the state is always
// "inactive", control methods are no-ops and the text methods return the
// same informational strings platform_utils.py did.
type NoService struct {
	Events events.Events
}

// NewService returns the no-op service for this host.
func NewService(Runner) Service { return &NoService{} }

// NewServiceWithEvents is NewService with a log sink.
func NewServiceWithEvents(_ Runner, ev events.Events) Service { return &NoService{Events: ev} }

// Name implements Service: "ClockworkOrangeService" on Windows, "" on macOS.
func (*NoService) Name() string {
	if runtime.GOOS == "windows" {
		return "ClockworkOrangeService"
	}
	return ""
}

// IsActive implements Service.
func (*NoService) IsActive(context.Context) ServiceState { return StateInactive }

// StatusDetails implements Service.
func (*NoService) StatusDetails(context.Context) string {
	switch runtime.GOOS {
	case "windows":
		return "Windows mode uses System Tray app, not a background service."
	case "darwin":
		return "macOS mode uses the GUI app with system tray. No background service."
	default:
		return "No background service on " + runtime.GOOS + "."
	}
}

// Start implements Service.
func (*NoService) Start(context.Context) error { return nil }

// Stop implements Service.
func (*NoService) Stop(context.Context) error { return nil }

// Restart implements Service.
func (*NoService) Restart(context.Context) error { return nil }

// Install implements Service.
func (s *NoService) Install(context.Context) error {
	switch runtime.GOOS {
	case "windows":
		s.Events.Infof("Service installation is not used on Windows. Use the Tray App.")
	case "darwin":
		s.Events.Infof("Service installation is not used on macOS. Use the GUI app.")
	}
	return nil
}

// Uninstall implements Service.
func (*NoService) Uninstall(context.Context) error { return nil }

// Logs implements Service.
func (*NoService) Logs(context.Context, int) string {
	switch runtime.GOOS {
	case "windows":
		return "Check console output or %TEMP% for logs."
	case "darwin":
		return "Check the Activity Log in the GUI."
	default:
		return "No service logs on " + runtime.GOOS + "."
	}
}
