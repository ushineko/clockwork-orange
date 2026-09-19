package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/container"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"
)

// arriverFor is the section with this title, as something the shell can tell
// it has been arrived at.
func arriverFor(t *testing.T, u *ui, title string) shell.Arriver {
	t.Helper()
	for _, sec := range sections(u) {
		if sec.Title() != title {
			continue
		}
		a, ok := sec.(shell.Arriver)
		require.Truef(t, ok, "%q does not take an arrival", title)
		return a
	}
	t.Fatalf("no section titled %q", title)
	return nil
}

/*
Arriving at the Blacklist reads it again; rebuilding it where it stands does
not (spec 013 AC12).

The distinction is the whole point of the hook. A plugin run marks an image
while the user is looking at another section, so what the Blacklist last read
is stale by the time they walk over to it -- and a section that refetched on
every build would loop, because the fetch finishing rebuilds the section that
started it.
*/
func TestArrivingAtTheBlacklistReadsItAgainAndARebuildDoesNot(t *testing.T) {
	u, _, _ := testUI(t)
	u.buildBlacklist()
	require.Empty(t, u.blacklist)

	require.NoError(t, u.deps.Blacklist.Add("deadbeef", "wallhaven", "/tmp/x.jpg"))

	u.buildBlacklist()
	require.Empty(t, u.blacklist, "a rebuild in place shows what was last read")

	arriverFor(t, u, sectionBlacklist).Arrive()
	u.buildBlacklist()
	require.Len(t, u.blacklist, 1, "arriving reads the store again")
	require.Equal(t, "deadbeef", u.blacklist[0].Hash)
}

// The same for History, whose statistics the CLI can change behind the
// window's back (spec 013 AC12).
func TestArrivingAtHistoryReadsItAgain(t *testing.T) {
	u, _, _ := testUI(t)
	u.buildHistory()
	require.True(t, u.histOK)

	arriverFor(t, u, sectionHistory).Arrive()
	require.False(t, u.histOK, "arriving drops the loaded flag, so the builder fetches")
}

// A section with nothing to refetch is not given an arrival hook, so the shell
// has nothing to call.
func TestSectionsWithNothingToRefetchTakeNoArrival(t *testing.T) {
	u, _, _ := testUI(t)
	for _, sec := range sections(u) {
		if sec.Title() == sectionBlacklist || sec.Title() == sectionHistory {
			continue
		}
		f, ok := sec.(*shell.FuncSection)
		require.True(t, ok)
		f.Arrive() // a no-op; this pins that it is safe to call on every section
	}
}

/*
The log sits under a divider the user can drag, and Service and Activity share
one position (spec 013 AC8, AC9).

Two sections with one key on purpose: the pane is one pane as far as the user
is concerned, and which of the two sections they see depends only on whether
the machine has systemd.
*/
func TestTheLogPaneSitsUnderASharedDivider(t *testing.T) {
	u, _, _ := testUI(t)

	svc, ok := u.buildService().(*container.Split)
	require.True(t, ok, "the Service section is a split, not a fixed stack")
	require.InDelta(t, logSplitOffset, svc.Offset, 0.001)

	// Dragged in Service, and the Activity section opens where it was left.
	svc.SetOffset(0.3)
	act, ok := u.buildActivity().(*container.Split)
	require.True(t, ok, "the Activity section is a split too")
	require.InDelta(t, 0.3, act.Offset, 0.001, "one key, one position")
}

// The Service section's controls and its buttons are still reachable now that
// the section is a split rather than a stack (spec 013 R4.1).
func TestTheServiceSectionsControlsSurviveTheSplit(t *testing.T) {
	u, _, runner := testUI(t)
	runner.Stdout = "inactive\n"
	body := u.buildService()
	require.NotNil(t, findButton(body, "Start"))
	require.NotNil(t, findButton(body, "Refresh now"))
	require.NotNil(t, findCheck(body), "the auto-refresh toggle")
}

/*
The controls whose consequence the label cannot fit carry a hover note, and the
notes already in the sections are still there (spec 013 AC11).

A tip is read by whoever hovers over the control; a note by whoever reads the
section. Neither replaces the other, so this pins both.
*/
func TestTheControlsThatNeedOneCarryATip(t *testing.T) {
	u, _, _ := testUI(t)

	svc := fynetest.Tips(u.buildService())
	require.Len(t, svc, 2, "Install and Uninstall, and not the three that do what they say")
	require.Contains(t, strings.Join(svc, "\n"), "systemd user unit")
	require.Contains(t, strings.Join(svc, "\n"), "are not touched")

	hist := u.buildHistory()
	require.Len(t, fynetest.Tips(hist), 2, "the two actions")
	require.Contains(t, strings.Join(fynetest.Texts(hist), "\n"), "Resetting the history",
		"the section's own note is still there")

	require.Len(t, fynetest.Tips(u.buildBlacklist()), 1, "Remove selected")
	require.Len(t, fynetest.Tips(u.buildAppearance()), 1, "the interface scale")
}
