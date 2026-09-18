package gui

import (
	"testing"

	"github.com/stretchr/testify/require"
	fd "github.com/ushineko/fynedesygn"

	"github.com/ushineko/clockwork-orange/internal/platform"
)

// The enablement matrix from service_manager.py:230-299, row for row (R7.4).
func TestServiceEnablementMatchesServiceManager(t *testing.T) {
	cases := map[platform.ServiceState]serviceButtons{
		platform.StateActive:       {start: false, stop: true, restart: true, install: false, uninstall: false},
		platform.StateInactive:     {start: true, stop: false, restart: false, install: true, uninstall: true},
		platform.StateActivating:   {start: false, stop: true, restart: false, install: false, uninstall: false},
		platform.StateDeactivating: {start: false, stop: false, restart: false, install: false, uninstall: false},
		platform.StateFailed:       {start: true, stop: true, restart: true, install: true, uninstall: true},
		platform.StateUnknown:      {start: true, stop: true, restart: true, install: true, uninstall: true},
		"something-else":           {start: true, stop: true, restart: true, install: true, uninstall: true},
	}
	for state, want := range cases {
		require.Equalf(t, want, serviceEnablement(state), "state %q", state)
	}
	require.Equal(t, fd.StatusGood, serviceStatus(platform.StateActive))
	require.Equal(t, fd.StatusBad, serviceStatus(platform.StateInactive))
	require.Equal(t, fd.StatusBad, serviceStatus(platform.StateFailed))
	require.Equal(t, fd.StatusWarn, serviceStatus(platform.StateActivating))
	require.Equal(t, fd.StatusInfo, serviceStatus(platform.StateUnknown))
}

// The Service section's buttons come out enabled per the matrix for the state
// the fake systemctl reports.
func TestServiceSectionGatesItsButtonsByState(t *testing.T) {
	u, _, runner := testUI(t)
	runner.Stdout = "active\n"
	body := u.buildService()
	require.True(t, u.serviceOK)
	require.Equal(t, platform.StateActive, u.service.State)
	require.True(t, findButton(body, "Start").Disabled())
	require.False(t, findButton(body, "Stop").Disabled())
	require.True(t, findButton(body, "Install").Disabled())
	require.True(t, findButton(body, "Uninstall").Disabled())

	runner.Stdout = "inactive\n"
	u.serviceOK = false
	body = u.buildService()
	require.False(t, findButton(body, "Start").Disabled())
	require.True(t, findButton(body, "Stop").Disabled())
	require.False(t, findButton(body, "Install").Disabled())
}
