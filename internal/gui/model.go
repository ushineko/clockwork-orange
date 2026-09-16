package gui

import (
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/platform"
)

// The types and mappings in this file are the presentation layer's own: they
// say how to paint what core returns, which is a different question from what
// core returns. Keeping them apart is what stops a rendering decision from
// landing in the package both front ends share.

// Status ranks a value so a card or a table row can be read at a glance rather
// than parsed word by word.
type Status int

const (
	// StatusInfo is a plain fact with no judgement attached.
	StatusInfo Status = iota
	// StatusGood is a state the user wants to be in.
	StatusGood
	// StatusWarn is a state that needs an action but has broken nothing yet.
	StatusWarn
	// StatusBad is a state that is already costing the user something.
	StatusBad
)

// levelStatus ranks a core log line, which is what colours the log panes.
func levelStatus(l events.Level) Status {
	switch l {
	case events.LevelWarn:
		return StatusWarn
	case events.LevelError:
		return StatusBad
	case events.LevelDebug, events.LevelInfo:
		return StatusInfo
	}
	return StatusInfo
}

// serviceStatus ranks the systemd state the way service_manager.py coloured
// its status label (R7.4): active green, inactive and failed red, the two
// transitional states amber, anything else plain.
func serviceStatus(s platform.ServiceState) Status {
	switch s {
	case platform.StateActive:
		return StatusGood
	case platform.StateInactive, platform.StateFailed:
		return StatusBad
	case platform.StateActivating, platform.StateDeactivating:
		return StatusWarn
	case platform.StateUnknown:
		return StatusInfo
	}
	return StatusInfo
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
