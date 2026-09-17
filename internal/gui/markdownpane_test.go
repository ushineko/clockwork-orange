package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/gui/assets"
)

// paneInScroll is a pane holding the README, laid out inside a scroll the size
// of a section pane, which is how the About section uses it.
func paneInScroll(t *testing.T, w, h float32) (*markdownPane, *container.Scroll) {
	t.Helper()
	test.NewTempApp(t)
	p := newMarkdownPane(string(assets.README()))
	sc := container.NewScroll(container.NewVBox(p))
	win := test.NewWindow(sc)
	t.Cleanup(win.Close)
	win.Resize(fyne.NewSize(w, h))
	p.follow(sc)
	return p, sc
}

// liveBlocks is how many of the document's blocks have their text in the tree.
func liveBlocks(p *markdownPane) int {
	n := 0
	for _, live := range p.live {
		if live {
			n++
		}
	}
	return n
}

// documentHeight is the pane's height as the layout above it sees it.
func documentHeight(p *markdownPane) float32 { return p.MinSize().Height }

// scrollBy moves the scroll the way a wheel notch does, so that OnScrolled
// fires: assigning Offset does not.
func scrollBy(sc *container.Scroll, dy float32) {
	sc.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, dy)})
}

/*
The About README is longer than the window and used to be one RichText, so
every scroll step laid out and repainted all ~250 of its segments and the
section scrolled in fits and bursts (spec 011 AC1).
*/
func TestMarkdownPaneKeepsDistantBlocksOutOfTheWidgetTree(t *testing.T) {
	p, _ := paneInScroll(t, 900, 700)

	require.Greater(t, len(p.blocks), 20, "the README should split into many blocks")
	require.Less(t, liveBlocks(p), len(p.blocks),
		"a document taller than the viewport should not render all of itself at once")
	require.False(t, p.live[len(p.live)-1],
		"the last block is screens away from the top and should not be in the tree")
	require.True(t, p.live[0], "the first block is on screen and should be rendered")
}

// Scrolling has to bring text with it: a block that is reached must be
// rendered by the time it is on screen (spec 011 AC2).
func TestMarkdownPaneRendersBlocksAsTheyAreScrolledTo(t *testing.T) {
	p, sc := paneInScroll(t, 900, 700)

	last := len(p.blocks) - 1
	require.False(t, p.live[last])
	for i := 0; i < 200 && !p.live[last]; i++ {
		scrollBy(sc, -400)
	}
	require.True(t, p.live[last], "the end of the document should render once scrolled to")
	require.False(t, p.live[0], "the top of the document should have been released by then")
}

/*
Nothing transient may reflow the interface: releasing a block must not change
the document's height, or the scrollbar would jump under the hand that is
dragging it (spec 011 AC3).
*/
func TestMarkdownPaneHeightIsTheSameWhicheverBlocksAreRendered(t *testing.T) {
	p, sc := paneInScroll(t, 900, 700)

	before := documentHeight(p)
	require.Greater(t, before, float32(700), "the README should be taller than the viewport")
	for i := 0; i < 20; i++ {
		scrollBy(sc, -400)
	}
	require.NotEqual(t, liveBlocks(p), 0)
	require.Equal(t, before, documentHeight(p), "the document changed height while scrolling")
}

// A narrower pane wraps the same paragraphs into more lines, so the blocks are
// measured again on a resize rather than keeping the old heights (spec 011 AC4).
func TestMarkdownPaneMeasuresAgainWhenTheWindowNarrows(t *testing.T) {
	p, _ := paneInScroll(t, 900, 700)

	wide := documentHeight(p)
	p.Resize(fyne.NewSize(450, p.Size().Height))
	require.Greater(t, documentHeight(p), wide, "narrowing should make the document taller")
}

/*
The scroll outlives the section it was showing. A pane that kept its callback
after the section was replaced would render into a widget tree nobody is
looking at, on every scroll of every other section (spec 011 AC5).
*/
func TestMarkdownPaneDetachStopsWatchingTheScroll(t *testing.T) {
	p, sc := paneInScroll(t, 900, 700)

	require.NotNil(t, sc.OnScrolled)
	p.detach()
	require.Nil(t, sc.OnScrolled)
	require.Nil(t, p.view)
}

/*
Without a scroll to be outside of, the pane is a plain document: every block
renders. The headless tests build every section before the window has a shell,
so About is built with a nil content scroll, and a pane that assumed one would
take the window down on the first launch that got there first.
*/
func TestMarkdownPaneWithNoViewportRendersEverything(t *testing.T) {
	test.NewTempApp(t)
	p := newMarkdownPane("# One\n\nTwo\n\nThree")
	require.NotPanics(t, func() { p.follow(nil) })
	p.Resize(fyne.NewSize(400, 100))

	require.Equal(t, 3, len(p.blocks))
	require.Equal(t, 3, liveBlocks(p))
}

// A blank line inside a fenced code block is part of the code, not a paragraph
// break: splitting there would render the fence as two broken halves.
func TestMarkdownBlocksKeepFencedCodeWhole(t *testing.T) {
	blocks := markdownBlocks("intro\n\n```\nfirst\n\nsecond\n```\n\nafter")

	require.Equal(t, []string{"intro", "```\nfirst\n\nsecond\n```", "after"}, blocks)
}

// The blocks are the document: rendering them in order must not drop or
// reorder any of its text.
func TestMarkdownBlocksKeepEveryLineOfTheDocument(t *testing.T) {
	src := string(assets.README())

	var got []string
	for _, b := range markdownBlocks(src) {
		got = append(got, strings.Split(b, "\n")...)
	}
	var want []string
	for _, line := range strings.Split(src, "\n") {
		if strings.TrimSpace(line) != "" {
			want = append(want, line)
		}
	}
	require.Equal(t, want, got)
}

// scrollableIn reports whether o, or anything it draws, scrolls. Fyne hands a
// wheel event to the innermost scrollable under the pointer and does not pass
// it on, so one of these inside the document is one place the page stops.
func scrollableIn(o fyne.CanvasObject) bool {
	if _, ok := o.(fyne.Scrollable); ok {
		return true
	}
	switch v := o.(type) {
	case *fyne.Container:
		for _, child := range v.Objects {
			if scrollableIn(child) {
				return true
			}
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(v).Objects() {
			if scrollableIn(child) {
				return true
			}
		}
	}
	return false
}

/*
The wheel belongs to the section. The README is mostly indented commands, and
Fyne draws a code block inside a horizontal scroll, which swallows the wheel
and turns a vertical notch into a sideways one — so the About section stopped
dead wherever the pointer rested on a command (spec 011 AC11).
*/
func TestAboutDocumentHasNothingInItThatTakesTheWheel(t *testing.T) {
	test.NewTempApp(t)
	p := newMarkdownPane(string(assets.README()))
	p.Resize(fyne.NewSize(900, 700))

	for i := range p.blocks {
		require.Falsef(t, scrollableIn(p.visual(i)),
			"block %d renders something that takes the wheel:\n%s", i, p.blocks[i])
	}
}

/*
A canary, not a contract: this is the Fyne behaviour codePanel exists to avoid
(Fyne 2.8's richCodeBlock wraps its label in an HScroll). If this ever fails,
Fyne has stopped scrolling code blocks and the panel can go.
*/
func TestFyneStillDrawsMarkdownCodeInsideAScroll(t *testing.T) {
	test.NewTempApp(t)
	rt := widget.NewRichTextFromMarkdown("    make build\n")
	rt.Resize(fyne.NewSize(400, 100))

	require.True(t, scrollableIn(rt))
}

// Both of Markdown's code spellings are code, and prose is not.
func TestCodeBlockTextStripsFencesAndIndents(t *testing.T) {
	fenced, ok := codeBlockText("```sh\nmake build\n```")
	require.True(t, ok)
	require.Equal(t, "make build", fenced)

	indented, ok := codeBlockText("    make build\n    make test")
	require.True(t, ok)
	require.Equal(t, "make build\nmake test", indented)

	_, ok = codeBlockText("A paragraph that happens to mention    spaces.")
	require.False(t, ok)

	_, ok = codeBlockText("- a list item\n  continued here")
	require.False(t, ok)
}

// A code panel keeps the code: wrapping it must not drop or reorder a line.
func TestCodePanelShowsEveryLineOfTheCode(t *testing.T) {
	test.NewTempApp(t)
	code := "clockwork-orange -f ~/Pictures/wallpaper.jpg\nclockwork-orange --lockscreen -d ~/Pictures"
	c := newCodePanel(code)
	c.Resize(fyne.NewSize(600, 100))

	var texts []string
	for _, o := range test.WidgetRenderer(c).Objects() {
		texts = append(texts, textIn(o)...)
	}
	require.Contains(t, strings.Join(texts, "\n"), code)
}

// textIn is every string a rendered object draws, in order.
func textIn(o fyne.CanvasObject) []string {
	switch v := o.(type) {
	case *canvas.Text:
		return []string{v.Text}
	case *widget.Label:
		return []string{v.Text}
	case *fyne.Container:
		var out []string
		for _, child := range v.Objects {
			out = append(out, textIn(child)...)
		}
		return out
	case fyne.Widget:
		var out []string
		for _, child := range test.WidgetRenderer(v).Objects() {
			out = append(out, textIn(child)...)
		}
		return out
	}
	return nil
}
