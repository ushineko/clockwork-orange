package gui

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"
)

/*
The About section renders the embedded README through the library's document
pane, and nothing in it takes the wheel from the section: Fyne draws a Markdown
code block inside a horizontal scroll, and the README is mostly commands, so
the page used to stop dead wherever the pointer rested on one (spec 011). The
pane's own behaviour is tested in the library; this pins that the section
uses it.
*/
func TestAboutRendersTheReadmeWithNothingThatTakesTheWheel(t *testing.T) {
	u, _, _ := testUI(t)
	body := u.buildAbout()
	require.NotNil(t, u.readme)
	require.Greater(t, u.readme.Blocks(), 20, "the README should split into many blocks")
	require.Contains(t, fynetest.Text(body), "Clockwork Orange")
	require.False(t, fynetest.ScrollableIn(body), "something in the About section takes the wheel")
}
