// This project's own: nmsbonker has no long-document section, so there is
// nothing upstream to keep in sync with.

package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

/*
A markdown pane: a document longer than the window, rendered a screenful at a
time.

Fyne's RichText lays out and repaints every segment it holds on each refresh,
and a scroll refreshes its content as it moves. The README is 44 blocks and
some 250 segments, so the About section was paying for the whole document on
every wheel notch and scrolled in fits and bursts (spec 011).

The pane fixes that by keeping only the blocks near the viewport in the
widget tree. Each block gets a transparent spacer sized to the height that
block needs at the current width; the rendered text is laid on top of the
spacer when the block comes near the viewport and taken away again when it
goes far from it. The spacer stays either way, so the document's height and
every block's position are the same whether or not the text is currently
rendered: scrolling never reflows the interface and the scrollbar never
jumps.

The measuring pass builds each block's RichText once, which is the parse the
section used to do anyway. What it does not do is leave 250 segments in the
tree for the toolkit to walk on every frame.

Use it for any document that can outgrow the window — the About README is the
one today. A short, fixed paragraph is a wrapped label; a stream of lines that
arrives while the window is open is a logPane.
*/
/*
mdOverscan is how much of a viewport's worth of document is kept rendered
above and below what is on screen, so that a fast scroll never reaches a block
before the block is rendered.

Half a screen of runway either side is several wheel notches, and it is what
the cost is paid for: with the About README in a 900x700 pane, a frame costs
about a third less than the same document as one RichText (spec 011 AC6). A
larger margin renders more of the document than anyone is looking at; a
smaller one saves little more and starts to matter when the scrollbar is
thrown.
*/
const mdOverscan = 0.5

type markdownPane struct {
	widget.BaseWidget

	// blocks is the document split into paragraph-sized pieces, in order.
	blocks []string
	// vis[i] is block i rendered; built by the measuring pass and kept, so a
	// block that scrolls back into view is not parsed again.
	vis []fyne.CanvasObject
	// spacers[i] holds block i's height whether or not its text is in the
	// tree; cells[i] is what the document's column actually contains.
	spacers []*canvas.Rectangle
	cells   []*fyne.Container
	// live[i] says block i's text is currently in the tree.
	live []bool

	// body is the column of cells, and the widget's only child.
	body *fyne.Container
	// view is the scroll this pane is inside, watched for movement. Nil means
	// nobody is scrolling it, and then every block is rendered: a pane with no
	// viewport to be outside of is a plain document.
	view *container.Scroll
	// width is the width the blocks were last measured at.
	width float32
}

/*
newMarkdownPane renders src, which is Markdown.

The pane is not scrollable itself. It is meant to go inside a section's scroll
and be handed that scroll with follow, so that one scrollbar governs the
document and whatever the section puts above it.
*/
func newMarkdownPane(src string) *markdownPane {
	p := &markdownPane{blocks: markdownBlocks(src)}
	p.vis = make([]fyne.CanvasObject, len(p.blocks))
	p.spacers = make([]*canvas.Rectangle, len(p.blocks))
	p.cells = make([]*fyne.Container, len(p.blocks))
	p.live = make([]bool, len(p.blocks))

	cells := make([]fyne.CanvasObject, len(p.blocks))
	for i := range p.blocks {
		p.spacers[i] = canvas.NewRectangle(color.Transparent)
		p.cells[i] = container.NewStack(p.spacers[i])
		cells[i] = p.cells[i]
	}
	p.body = container.NewVBox(cells...)
	p.ExtendBaseWidget(p)
	return p
}

/*
follow watches sc for movement, so the pane can render the blocks the user is
looking at. Call detach before the section holding the pane is replaced: the
scroll outlives it.

A nil scroll is not an error: a section built before the window's shell exists
(which is how the headless tests build every section) has no viewport, and a
pane with no viewport renders the whole document.
*/
func (p *markdownPane) follow(sc *container.Scroll) {
	if sc == nil {
		p.sync()
		return
	}
	p.view = sc
	sc.OnScrolled = func(fyne.Position) { p.sync() }
	p.sync()
}

// detach gives the scroll back: its callback would otherwise keep calling into
// a pane that is no longer on screen.
func (p *markdownPane) detach() {
	if p.view != nil {
		p.view.OnScrolled = nil
		p.view = nil
	}
}

func (p *markdownPane) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.body)
}

// Resize measures the blocks again when the width changes — a narrower window
// wraps the same paragraph into more lines — and then renders what is in view.
func (p *markdownPane) Resize(s fyne.Size) {
	p.BaseWidget.Resize(s)
	p.measure(s.Width)
	p.sync()
}

/*
measure sizes every block's spacer to the height that block needs at width w.

It is the one place the document's shape is decided. Blocks are measured, not
estimated: an estimate that came out short would clip the last line of a
paragraph, and one that came out long would leave a gap, and both would be
visible only on the machines whose font differs from the one that was guessed
against.
*/
func (p *markdownPane) measure(w float32) {
	if w <= 0 || w == p.width {
		return
	}
	p.width = w
	for i := range p.blocks {
		o := p.visual(i)
		o.Resize(fyne.NewSize(w, o.MinSize().Height))
		p.spacers[i].SetMinSize(fyne.NewSize(0, o.MinSize().Height))
	}
	p.body.Refresh()
}

// visual is block i rendered, built on first use.
func (p *markdownPane) visual(i int) fyne.CanvasObject {
	if p.vis[i] == nil {
		p.vis[i] = renderMarkdownBlock(p.blocks[i])
	}
	return p.vis[i]
}

/*
renderMarkdownBlock renders one block: prose through Fyne's Markdown widget,
code through this package's own panel.

Code is not left to the Markdown widget because since Fyne 2.8 it draws a code
block as a label inside a horizontal scroll. A scroll takes the wheel from
whatever is under the pointer and does not pass it on, and when the code is
wider than the pane and the block is not taller than it, the scroller turns a
vertical wheel notch into a horizontal one (internal/widget/scroller.go,
scrollBy). The README is mostly indented commands, so the About section stopped
dead wherever the pointer happened to rest on one, and scrolling the section
came in fits and bursts (spec 011).
*/
func renderMarkdownBlock(src string) fyne.CanvasObject {
	if code, ok := codeBlockText(src); ok {
		return newCodePanel(code)
	}
	rt := widget.NewRichTextFromMarkdown(src)
	rt.Wrapping = fyne.TextWrapWord
	return rt
}

/*
sync puts the blocks near the viewport in the tree and takes the rest out.

"Near" is one viewport's worth above and below what is on screen, so a fling
does not outrun the rendering: by the time a block is on screen it has been in
the tree since it was a screenful away.
*/
func (p *markdownPane) sync() {
	if p.view == nil {
		for i := range p.blocks {
			p.render(i, true)
		}
		return
	}
	// The pane's own coordinates: where the viewport is over this widget, not
	// over the scroll's content, which also holds whatever is above the pane.
	viewH := p.view.Size().Height
	over := viewH * mdOverscan
	top := p.view.Offset.Y - p.Position().Y - over
	bottom := p.view.Offset.Y - p.Position().Y + viewH + over

	var y float32
	for i := range p.blocks {
		h := p.spacers[i].MinSize().Height
		p.render(i, y+h >= top && y <= bottom)
		y += h + theme.Padding()
	}
}

// render puts block i's text in the tree, or takes it out. The spacer stays in
// both cases: the block keeps its height and the document its shape.
func (p *markdownPane) render(i int, want bool) {
	if p.live[i] == want {
		return
	}
	p.live[i] = want
	if want {
		p.cells[i].Objects = []fyne.CanvasObject{p.spacers[i], p.visual(i)}
	} else {
		p.cells[i].Objects = []fyne.CanvasObject{p.spacers[i]}
	}
	p.cells[i].Refresh()
}

/*
markdownBlocks splits a Markdown document into the pieces that are rendered
independently.

A blank line is the split, which is Markdown's own paragraph rule: headings,
paragraphs, lists and indented code blocks all end at one. Fenced code is the
exception — a blank line inside ``` is part of the code — so fences are
tracked and not split on.
*/
func markdownBlocks(src string) []string {
	var (
		blocks []string
		cur    []string
		fenced bool
	)
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			cur = append(cur, line)
			if !fenced {
				flush()
			}
			continue
		}
		if !fenced && strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}

/*
codeBlockText reports whether a block is code, and gives back the code with
its fence or its indent taken off.

Both of Markdown's spellings count: a fenced block, and a run of lines each
indented by four spaces or a tab, which is what this project's README uses for
every command it shows.
*/
func codeBlockText(src string) (string, bool) {
	lines := strings.Split(src, "\n")
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		body := lines[1:]
		if n := len(body); n > 0 && strings.HasPrefix(strings.TrimSpace(body[n-1]), "```") {
			body = body[:n-1]
		}
		return strings.Join(body, "\n"), true
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.TrimSpace(line) == "":
			out = append(out, "")
		case strings.HasPrefix(line, "    "):
			out = append(out, line[4:])
		case strings.HasPrefix(line, "\t"):
			out = append(out, line[1:])
		default:
			return "", false
		}
	}
	return strings.Join(out, "\n"), true
}

/*
A code panel: monospace text on the same panel Fyne draws a code block on,
without the horizontal scroll that takes the wheel from the section.

Long lines wrap rather than run off the side. Fyne's own block scrolls them
sideways, which reads better but puts the rest of the line behind a gesture
that is exactly the one the page needs; here the whole command is on screen
and the wheel always belongs to the document.
*/
type codePanel struct {
	widget.BaseWidget
	code string
	bg   *canvas.Rectangle
}

func newCodePanel(code string) *codePanel {
	c := &codePanel{code: code}
	c.ExtendBaseWidget(c)
	return c
}

func (c *codePanel) CreateRenderer() fyne.WidgetRenderer {
	c.bg = canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	c.bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	c.bg.StrokeWidth = 1
	c.bg.CornerRadius = theme.Size(theme.SizeNameInputRadius)

	text := widget.NewLabelWithStyle(c.code, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	text.Wrapping = fyne.TextWrapWord
	return widget.NewSimpleRenderer(container.NewStack(c.bg, container.NewPadded(text)))
}

// Refresh repaints the panel from the active scheme, so that changing the
// colours in Appearance does not leave a block drawn in the old ones.
func (c *codePanel) Refresh() {
	if c.bg != nil {
		c.bg.FillColor = theme.Color(theme.ColorNameInputBackground)
		c.bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	}
	c.BaseWidget.Refresh()
}
