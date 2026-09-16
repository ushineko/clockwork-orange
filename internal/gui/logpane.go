// Copied from nmsbonker (same author) — keep in sync by hand. The log model and
// the follow-tail rule are nmsbonker's buildlog.go without the step list.

package gui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/events"
)

/*
A log pane: a fixed-height list of monospace lines that scrolls itself to the
end unless the user has scrolled up to read something.

Two rules shape the code below. The log is written from core's worker
goroutines and read on the UI thread, so it is behind a mutex and the widget
is refreshed on a timer rather than per line — a hundred fyne.Do calls a
second would make the window slower than the plugin. And the pane never
grows: it is a fixed-height list inside a section that keeps its shape,
because nothing transient may reflow the interface.

The Service section fills one from the journal tail, the Activity section
from the in-process timer, and the run dialog from a plugin's events (R7.4,
R7.5).
*/

// maxLogLines is how much output is retained: activity_log.py kept 1000.
const maxLogLines = 1000

// logDropChunk is how many of the oldest lines go at once when the cap is hit.
const logDropChunk = 128

// logPaneHeight is the pane's fixed height.
const logPaneHeight = 360

// logPumpInterval is how often a pane redraws while lines arrive.
const logPumpInterval = 100 * time.Millisecond

// logLine is one line and how to paint it.
type logLine struct {
	level events.Level
	text  string
}

// logModel holds the lines. Safe to append to from any goroutine.
type logModel struct {
	mu      sync.Mutex
	lines   []logLine
	dropped int
	dirty   bool
}

// append adds a message, one row per line of it, dropping the oldest chunk
// when the cap is reached.
func (m *logModel) append(level events.Level, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if len(m.lines) >= maxLogLines {
			drop := min(logDropChunk, len(m.lines))
			m.lines = append(m.lines[:0], m.lines[drop:]...)
			m.dropped += drop
		}
		m.lines = append(m.lines, logLine{level: level, text: strings.TrimRight(line, "\r")})
	}
	m.dirty = true
}

// replace swaps the whole content, for a journal tail re-read.
func (m *logModel) replace(text string) {
	m.mu.Lock()
	m.lines, m.dropped = nil, 0
	m.mu.Unlock()
	m.append(events.LevelInfo, text)
}

// reset empties the log.
func (m *logModel) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines, m.dropped, m.dirty = nil, 0, true
}

func (m *logModel) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.lines)
}

func (m *logModel) droppedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropped
}

// at returns one line, or the zero line for an index that has since been
// dropped: the widget's item count and its update callback are read at
// different moments, so an out-of-range index is expected rather than a bug.
func (m *logModel) at(i int) logLine {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.lines) {
		return logLine{}
	}
	return m.lines[i]
}

// takeDirty reports whether anything changed since the last call, and clears
// the flag.
func (m *logModel) takeDirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	was := m.dirty
	m.dirty = false
	return was
}

// text renders the whole log for the clipboard.
func (m *logModel) text() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	if m.dropped > 0 {
		b.WriteString("… ")
		b.WriteString(strconv.Itoa(m.dropped))
		b.WriteString(" earlier line(s) were dropped; the log keeps the last ")
		b.WriteString(strconv.Itoa(maxLogLines))
		b.WriteString(".\n")
	}
	for _, l := range m.lines {
		b.WriteString(l.text)
		b.WriteString("\n")
	}
	return b.String()
}

/*
followTail decides whether the pane should keep scrolling to the end.

Fyne's list has no "the user scrolled" callback, so this compares the offset the
last auto-scroll left behind with the offset the list is at now. Lower means the
user dragged the pane up to read something, and a pane that yanks itself back to
the bottom half a second later is unusable. Following resumes when they scroll
back down, or when they tick the box.

The tolerance is there because ScrollToBottom lands on a fractional offset and
the list re-measures as rows arrive; without it the pane stops following itself.
*/
func followTail(following bool, current, wanted float32) bool {
	const tolerance = 4
	if !following {
		return false
	}
	return current >= wanted-tolerance
}

/*
logPane is a log model plus the live widgets drawing it, non-nil only while a
section holding it is on screen. The widgets are updated in place: rebuilding
the section on every line is exactly the reflow the project rule forbids.
*/
type logPane struct {
	log *logModel

	// follow is the auto-scroll state: true until the user scrolls up.
	follow bool
	// wantOffset is the scroll offset left behind by the last auto-scroll.
	wantOffset float32

	list      *widget.List
	counter   *widget.Label
	followBox *widget.Check
	stamp     *widget.Label
	updated   time.Time
}

func newLogPane() *logPane { return &logPane{log: &logModel{}, follow: true} }

// detach forgets the live widgets. Called when the content pane is replaced.
func (p *logPane) detach() {
	p.list, p.counter, p.followBox, p.stamp = nil, nil, nil, nil
	// The next list starts at the top, so the offset the last automatic scroll
	// left behind describes a widget that no longer exists.
	p.wantOffset = 0
}

// events is a sink that writes every level into the pane, timestamped the way
// activity_log.py did (HH:MM:SS [LEVEL] message).
func (p *logPane) events() events.Events {
	return events.Events{
		OnLog: func(level events.Level, msg string) {
			p.log.append(level, time.Now().Format("15:04:05")+" "+level.String()+" "+msg)
		},
	}
}

// widget builds the pane: a header row with the counter, the follow box and
// Copy / Clear, over the fixed-height list. onClear is nil for a pane whose
// content comes from the journal rather than from this process.
func (p *logPane) widget(u *ui, title string, onClear func()) fyne.CanvasObject {
	log := p.log
	list := widget.NewList(
		func() int { return log.len() },
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			line := log.at(i)
			l := o.(*widget.Label)
			// Importance first: SetText refreshes, and the refresh is what
			// applies it. See the note in views_table.go.
			l.Importance = importanceFor(levelStatus(line.level))
			l.SetText(line.text)
		},
	)
	p.list = list

	p.counter = widget.NewLabel("")
	p.counter.Importance = widget.LowImportance
	p.stamp = widget.NewLabel("")
	p.stamp.Importance = widget.LowImportance

	follow := widget.NewCheck("Follow the tail", func(b bool) {
		p.follow = b
		if b {
			p.draw()
		}
	})
	follow.SetChecked(p.follow)
	p.followBox = follow

	copyLog := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		u.app.Clipboard().SetContent(log.text())
		u.flash(fmt.Sprintf("Copied %d line(s) to the clipboard.", log.len()), StatusGood)
	})
	controls := container.NewHBox(p.counter, follow, copyLog)
	if onClear != nil {
		controls.Add(widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
			log.reset()
			onClear()
			p.draw()
		}))
	}
	bar := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		controls, widget.NewLabel(""))
	p.draw()
	return container.NewBorder(bar, p.stamp, nil, nil, fixedHeight(list, logPaneHeight))
}

/*
draw refreshes the pane and decides whether to stay at the end. Called on the
UI thread.

The offset dance is the whole of the auto-scroll rule. See followTail.
*/
func (p *logPane) draw() {
	if p.list == nil {
		return
	}
	if p.counter != nil {
		text := fmt.Sprintf("%d line(s)", p.log.len())
		if d := p.log.droppedCount(); d > 0 {
			text = fmt.Sprintf("%d line(s), %d older dropped", p.log.len(), d)
		}
		p.counter.SetText(text)
	}
	if p.stamp != nil && !p.updated.IsZero() {
		p.stamp.SetText(fmt.Sprintf("Last updated %s | %d entries", p.updated.Format("15:04:05"), p.log.len()))
	}
	p.follow = followTail(p.follow, p.list.GetScrollOffset(), p.wantOffset)
	// The box is the state, so it has to say what the state is.
	if p.followBox != nil && p.followBox.Checked != p.follow {
		p.followBox.SetChecked(p.follow)
	}
	p.list.Refresh()
	if !p.follow {
		return
	}
	p.list.ScrollToBottom()
	p.wantOffset = p.list.GetScrollOffset()
}

// touch records that content arrived, for the "Last updated" stamp.
func (p *logPane) touch() { p.updated = time.Now() }

/*
pump redraws the pane on a timer until stop is called, then once more, so the
last lines are on screen even if they arrived between ticks. The quit channel
is what makes the wait terminate: stopping a ticker does not close its
channel.
*/
func (p *logPane) pump() (stop func()) {
	ticker := time.NewTicker(logPumpInterval)
	quit := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		defer ticker.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				if !p.log.takeDirty() {
					continue
				}
				fyne.Do(func() { p.touch(); p.draw() })
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(quit)
			<-stopped
			fyne.Do(func() { p.touch(); p.draw() })
		})
	}
}
