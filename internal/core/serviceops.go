package core

import (
	"context"

	"github.com/ushineko/clockwork-orange/internal/platform"
)

// ServiceStatusResult is what the Service section and `service status` show.
type ServiceStatusResult struct {
	Name    string
	State   platform.ServiceState
	Details string
}

// ServiceStatus reports the daemon's systemd state (R4.5).
func ServiceStatus(ctx context.Context, req Request) ServiceStatusResult {
	d := req.deps()
	d.ensurePlatform(req.Events)
	return ServiceStatusResult{
		Name:    d.Service.Name(),
		State:   d.Service.IsActive(ctx),
		Details: d.Service.StatusDetails(ctx),
	}
}

// ServiceStart starts the user unit.
func ServiceStart(ctx context.Context, req Request) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	return d.Service.Start(ctx)
}

// ServiceStop stops the user unit.
func ServiceStop(ctx context.Context, req Request) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	return d.Service.Stop(ctx)
}

// ServiceRestart restarts the user unit.
func ServiceRestart(ctx context.Context, req Request) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	return d.Service.Restart(ctx)
}

// ServiceInstall writes and enables the user unit.
func ServiceInstall(ctx context.Context, req Request) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	return d.Service.Install(ctx)
}

// ServiceUninstall stops, disables and removes the user unit.
func ServiceUninstall(ctx context.Context, req Request) error {
	d := req.deps()
	d.ensurePlatform(req.Events)
	return d.Service.Uninstall(ctx)
}

// ServiceLogsRequest asks for the last N journal lines.
type ServiceLogsRequest struct {
	Request
	Lines int
}

// ServiceLogs returns the journal tail (default 50 lines).
func ServiceLogs(ctx context.Context, req ServiceLogsRequest) string {
	d := req.deps()
	d.ensurePlatform(req.Events)
	if req.Lines <= 0 {
		req.Lines = 50
	}
	return d.Service.Logs(ctx, req.Lines)
}
