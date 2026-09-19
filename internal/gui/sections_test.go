package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
	"github.com/ushineko/fynedesygn/shell"

	"github.com/ushineko/clockwork-orange/internal/gui/assets"
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

// --- nothing important scrolls away, nothing long runs off (spec 013 R10-R12) ---

// longNote is how many characters make a label a paragraph rather than a
// caption. The shortest note in the window is comfortably under it and the
// shortest paragraph comfortably over.
const longNote = 120

/*
No section sets its minimum width from an unwrapped paragraph (spec 013 R12).

widgets.Dim is a caption: it does not wrap, so a paragraph in one makes the
section as wide as the whole unbroken line and the shell's scroller then scrolls
sideways instead of reflowing. widgets.DimWrapped is the paragraph. This walks
every section and fails on any long label that does not wrap.

Monospace labels are exempt: a log line or a systemctl detail is read as it was
written, and the scroller around it is the affordance on purpose.
*/
func TestNoSectionHasAnUnwrappedParagraph(t *testing.T) {
	u, _, _ := testUI(t)
	for _, sec := range sections(u) {
		title := sec.Title()
		walk(sec.Build(u.sh), func(o fyne.CanvasObject) bool {
			l, ok := o.(*widget.Label)
			if !ok || l.TextStyle.Monospace || len(l.Text) <= longNote {
				return true
			}
			require.NotEqualf(t, fyne.TextWrapOff, l.Wrapping,
				"%s: a %d-character note does not wrap, so the section is as wide as the line: %.60s…",
				title, len(l.Text), l.Text)
			return true
		})
	}
}

/*
Every plugin section has its own mark (spec 013 R10).

Three entries drawn with one picture icon are three things to guess between,
and in the icons-only navigation shape the mark is nearly all there is.
*/
func TestEachPluginSectionHasItsOwnIcon(t *testing.T) {
	u, _, _ := testUI(t)
	seen := map[string]string{}
	for _, sec := range sections(u) {
		name, isPlugin := pluginForTitle(sec.Title())
		if !isPlugin {
			continue
		}
		icon := sec.Icon()
		require.NotNilf(t, icon, "%s has no icon", sec.Title())
		other, clash := seen[icon.Name()]
		require.Falsef(t, clash, "%s and %s draw the same mark, %s", other, sec.Title(), icon.Name())
		seen[icon.Name()] = sec.Title()
		require.NotNilf(t, assets.PluginSVG(name), "%s has no drawing of its own", name)
	}
	require.Len(t, seen, 3, "local, wallhaven and duckduckgo_images")
}

// A plugin this build has no drawing for keeps the generic picture icon rather
// than none at all.
func TestAnUnknownPluginKeepsTheGenericIcon(t *testing.T) {
	u, _, _ := testUI(t)
	_ = u
	require.Nil(t, assets.PluginSVG("stable_diffusion"))
	require.Equal(t, theme.FileImageIcon().Name(), pluginIcon("stable_diffusion")().Name())
}

/*
The plugin section's actions do not scroll away from the form (spec 013 R11).

Wallhaven has eleven fields. With the actions last in one column, the two
buttons the section exists for sat below the fold of a default-sized window,
and the user who had just finished filling the form had nothing to press.
*/
func TestThePluginActionsDoNotScrollAway(t *testing.T) {
	u, _, _ := testUI(t)
	u.pluginTab = 0
	body := u.buildPlugin("wallhaven")

	require.NotNil(t, findButton(body, "Download now"), "the action is in the section")
	require.NotNil(t, findButton(body, "Reset & run"))

	scrolled := 0
	walk(body, func(o fyne.CanvasObject) bool {
		sc, ok := o.(*container.Scroll)
		if !ok {
			return true
		}
		scrolled++
		require.Nil(t, findButton(sc, "Download now"), "the actions must not be inside the scroller")
		require.Nil(t, findButton(sc, "Reset & run"))
		return true
	})
	require.Positive(t, scrolled, "the form scrolls under the actions")
}

// --- affixed controls (spec 014) ---------------------------------------------

/*
buttonsBelowAScroller is every button under o that some container.Scroll
encloses, by label.

A button in a scroller moves with what it scrolls, which is right for a control
that belongs to a row and wrong for one that belongs to the section.
*/
func buttonsBelowAScroller(o fyne.CanvasObject) map[string]bool {
	out := map[string]bool{}
	var visit func(o fyne.CanvasObject, scrolled bool)
	visit = func(o fyne.CanvasObject, scrolled bool) {
		if o == nil {
			return
		}
		if b, ok := o.(*widget.Button); ok && scrolled {
			out[b.Text] = true
		}
		switch c := o.(type) {
		case *fyne.Container:
			for _, child := range c.Objects {
				visit(child, scrolled)
			}
		case *container.Scroll:
			visit(c.Content, true)
		case *container.Split:
			visit(c.Leading, scrolled)
			visit(c.Trailing, scrolled)
		case *container.AppTabs:
			for _, item := range c.Items {
				visit(item.Content, scrolled)
			}
		}
	}
	visit(o, false)
	return out
}

// buttonWithPrefix finds a button whose label starts with prefix; three of the
// affixed labels carry a count that changes with the data.
func buttonWithPrefix(o fyne.CanvasObject, prefix string) *widget.Button {
	var found *widget.Button
	walk(o, func(c fyne.CanvasObject) bool {
		if b, ok := c.(*widget.Button); ok && strings.HasPrefix(b.Text, prefix) && found == nil {
			found = b
		}
		return true
	})
	return found
}

/*
No control that starts work scrolls out of view (spec 014).

The rule, and the bug that produced it: the Service section's five verbs sat in
the scrolling half of its split, under a heading, a status line and a
fixed-height details pane, and the default divider position put them off the
bottom of it. The window reported that the service was running and offered no
way to stop it. The plugin section had the same shape for the same reason, one
long form earlier.

This holds every section named in AffixedActions, so the next section laid out
as a column that happens to fit fails the build rather than the window.
*/
func TestTheAffixedControlsDoNotScroll(t *testing.T) {
	u, _, _ := testUI(t)
	u.pluginTab = 0
	for _, sec := range sections(u) {
		title := sec.Title()
		want, named := AffixedActions()[title]
		if !named {
			continue
		}
		body := sec.Build(u.sh)
		scrolled := buttonsBelowAScroller(body)
		for _, prefix := range want {
			b := buttonWithPrefix(body, prefix)
			require.NotNilf(t, b, "%s: no control labelled %q", title, prefix)
			require.Falsef(t, scrolled[b.Text],
				"%s: %q is inside a scroller, so it scrolls away from the section it acts on", title, b.Text)
		}
	}
}

// Every section that has controls worth affixing is named. A section added
// with a toolbar and no entry would pass the test above by not being looked at.
func TestEverySectionWithControlsIsNamed(t *testing.T) {
	u, _, _ := testUI(t)
	named := AffixedActions()
	for _, sec := range sections(u) {
		title := sec.Title()
		if _, ok := named[title]; ok {
			continue
		}
		switch title {
		case sectionActivity, sectionSettings, sectionAbout:
			// Activity's only control is the pane's own Clear, drawn in the
			// pane header beside Copy. Settings' Validate and Copy act on the
			// YAML box they sit above, and travel with it. About is a
			// document.
			continue
		}
		t.Fatalf("%s has no AffixedActions entry; name its controls or say here why it has none", title)
	}
}
