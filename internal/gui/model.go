package gui

import (
	"time"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"

	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/platform"
)

// The types and mappings in this file are the presentation layer's own: they
// say how to paint what core returns, which is a different question from what
// core returns. Keeping them apart is what stops a rendering decision from
// landing in the package both front ends share.

// logLevel maps a core log level onto the log pane's, which is what colours
// a line. The mapping lives here so the pane never learns a domain level.
func logLevel(l events.Level) logpane.Level {
	switch l {
	case events.LevelDebug:
		return logpane.Debug
	case events.LevelWarn:
		return logpane.Warn
	case events.LevelError:
		return logpane.Error
	case events.LevelInfo:
		return logpane.Info
	}
	return logpane.Info
}

// paneEvents is a sink that writes every level into a pane, timestamped the
// way activity_log.py did (HH:MM:SS [LEVEL] message).
func paneEvents(p *logpane.Pane) events.Events {
	return events.Events{
		OnLog: func(level events.Level, msg string) {
			p.Model().Append(logLevel(level), time.Now().Format("15:04:05")+" "+level.String()+" "+msg)
		},
	}
}

// serviceStatus ranks the systemd state the way service_manager.py coloured
// its status label (R7.4): active green, inactive and failed red, the two
// transitional states amber, anything else plain.
func serviceStatus(s platform.ServiceState) fd.Status {
	switch s {
	case platform.StateActive:
		return fd.StatusGood
	case platform.StateInactive, platform.StateFailed:
		return fd.StatusBad
	case platform.StateActivating, platform.StateDeactivating:
		return fd.StatusWarn
	case platform.StateUnknown:
		return fd.StatusInfo
	}
	return fd.StatusInfo
}

// serviceStateText is the status line's wording, from service_manager.py.
func serviceStateText(s platform.ServiceState) string {
	switch s {
	case platform.StateActive:
		return "Service is running"
	case platform.StateInactive:
		return "Service is stopped"
	case platform.StateActivating:
		return "Service is starting"
	case platform.StateDeactivating:
		return "Service is stopping"
	case platform.StateFailed:
		return "Service failed"
	case platform.StateUnknown:
		return "Service status unknown"
	}
	return "Service status unknown"
}

/*
serviceButtons is the enablement matrix from service_manager.py:230-299
(R7.4), one row per systemd state. Install and Uninstall are refused while the
unit is running or in transition: uninstalling a running unit stops it under
the user, and installing over one rewrites the file it is executing from.
*/
type serviceButtons struct {
	start, stop, restart, install, uninstall bool
}

func serviceEnablement(s platform.ServiceState) serviceButtons {
	switch s {
	case platform.StateActive:
		return serviceButtons{start: false, stop: true, restart: true, install: false, uninstall: false}
	case platform.StateInactive:
		return serviceButtons{start: true, stop: false, restart: false, install: true, uninstall: true}
	case platform.StateActivating:
		return serviceButtons{start: false, stop: true, restart: false, install: false, uninstall: false}
	case platform.StateDeactivating:
		return serviceButtons{start: false, stop: false, restart: false, install: false, uninstall: false}
	case platform.StateFailed, platform.StateUnknown:
		return serviceButtons{start: true, stop: true, restart: true, install: true, uninstall: true}
	}
	return serviceButtons{start: true, stop: true, restart: true, install: true, uninstall: true}
}
