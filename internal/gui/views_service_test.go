package gui

import (
	"testing"

	"github.com/stretchr/testify/require"

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
	require.Equal(t, StatusGood, serviceStatus(platform.StateActive))
	require.Equal(t, StatusBad, serviceStatus(platform.StateInactive))
	require.Equal(t, StatusBad, serviceStatus(platform.StateFailed))
	require.Equal(t, StatusWarn, serviceStatus(platform.StateActivating))
	require.Equal(t, StatusInfo, serviceStatus(platform.StateUnknown))
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

// The log pane keeps the reader's place: once they scroll up it stops
// following, and following resumes when they come back to the end (R7.4).
func TestLogPaneFollowsTheTailOnlyWhileTheReaderIsAtTheEnd(t *testing.T) {
	require.True(t, followTail(true, 100, 100))
	require.True(t, followTail(true, 98, 100), "within tolerance of the last auto-scroll")
	require.False(t, followTail(true, 40, 100), "the reader scrolled up")
	require.False(t, followTail(false, 100, 100), "unticked stays unticked")
	require.True(t, followTail(true, 120, 100), "past the wanted offset still counts as at the end")
}

// The journal refresh replaces the pane's content rather than appending, so a
// tail re-read is not the same lines twice; the model caps its length.
func TestLogModelReplaceAndCap(t *testing.T) {
	m := &logModel{}
	m.replace("a\nb\n")
	m.replace("c\nd\ne")
	require.Equal(t, 3, m.len())
	require.Equal(t, "c", m.at(0).text)
	require.Equal(t, "c\nd\ne\n", m.text())
	for i := 0; i < maxLogLines+10; i++ {
		m.append(0, "x")
	}
	require.LessOrEqual(t, m.len(), maxLogLines)
	require.Greater(t, m.droppedCount(), 0)
	require.True(t, m.takeDirty())
	require.False(t, m.takeDirty())
}
