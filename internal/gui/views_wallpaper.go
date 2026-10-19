package gui

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/imaging"
)

// --- What is on screen, and the hold on it (spec 026) --------------------------

// Thumbnail geometry: 16:9, small enough that three monitors and the lock
// screen sit in one strip at the default window width.
const (
	thumbW = 176
	thumbH = 99
)

/*
wallpaperCard is the status card of the Service section (Activity on Windows
and macOS): a tile per monitor and one for the lock screen, each a thumbnail
over the target's name and the file, and a line with the time of the next
change led by the two controls that act on it, Pause/Resume and Skip.

The controls live in this card, not with the service's Start and Stop: they
hold and change the wallpaper, whoever is rotating it, and beside the
service's verbs they read as controls of the service.

Tiles are one fixed size in a wrapping grid, the thumbnail has its size
whether it has loaded or not, and the next-change line is one label whose
text changes, so neither a load nor the countdown nor a pause reflows the
section. The full path and how long the image has been up are the tile's tip.
*/
func (u *ui) wallpaperCard() fyne.CanvasObject {
	u.loadRotation()
	r := u.rotation
	st := r.State
	var tiles []fyne.CanvasObject
	for i, s := range st.Desktop {
		tiles = append(tiles, u.shownTile(fmt.Sprintf("Monitor %d", i+1), s, r.Control.Desktop[i] == s.Path))
	}
	if st.Lockscreen != nil {
		tiles = append(tiles, u.shownTile("Lock screen", *st.Lockscreen, r.Control.Lockscreen == st.Lockscreen.Path))
	}
	u.nextChange = widget.NewLabel("")
	u.nextChange.TextStyle = fyne.TextStyle{Bold: true}
	u.drawNextChange(time.Now())
	// The controls lead the line and the countdown follows them, so they
	// read as one statement: "[Pause] [Skip] Next change in 2:41".
	footer := container.NewBorder(nil, nil, u.wallpaperControls(), nil, u.nextChange)
	if len(tiles) == 0 {
		return widgets.Card("Wallpaper",
			widgets.Dim("No wallpaper has been recorded yet. The next change records one; Skip makes it now."),
			footer)
	}
	line := widget.NewLabel("X").MinSize().Height
	grid := container.NewGridWrap(fyne.NewSize(thumbW, thumbH+2*line), tiles...)
	return widgets.Card("Wallpaper", grid, footer)
}

// shownTile is one target: thumbnail, name with pin marker, file.
func (u *ui) shownTile(name string, s core.Shown, pinned bool) fyne.CanvasObject {
	img := canvas.NewImageFromResource(nil)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(thumbW, thumbH))
	u.thumbnail(s.Path, img)
	title := name
	if pinned {
		title += "  ·  pinned"
	}
	head := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	head.Truncation = fyne.TextTruncateEllipsis
	file := widget.NewLabel(filepath.Base(s.Path))
	file.Truncation = fyne.TextTruncateEllipsis
	file.Importance = widget.LowImportance
	tip := s.Path
	if !s.Since.IsZero() {
		tip += "\nSince " + widgets.HumanAgo(s.Since)
	}
	return widgets.WithTip(container.NewBorder(img, nil, nil, nil, container.NewVBox(head, file)), tip)
}

// thumbnail fills img with a small copy of path, decoded off the UI thread
// and kept for as long as the path stays on screen.
func (u *ui) thumbnail(path string, img *canvas.Image) {
	if t, ok := u.thumbs[path]; ok {
		img.Image = t
		return
	}
	go func() {
		src, _, err := imaging.DecodeFile(path)
		if err != nil {
			return
		}
		small := imaging.CoverResizeCrop(src, thumbW*2, thumbH*2)
		fyne.Do(func() {
			if u.thumbs == nil {
				u.thumbs = map[string]image.Image{}
			}
			u.thumbs[path] = small
			img.Image = small
			img.Refresh()
		})
	}()
}

// forgetThumbs drops thumbnails of images no longer on screen.
func (u *ui) forgetThumbs() {
	keep := map[string]image.Image{}
	for _, s := range u.rotation.State.Desktop {
		if t, ok := u.thumbs[s.Path]; ok {
			keep[s.Path] = t
		}
	}
	if l := u.rotation.State.Lockscreen; l != nil {
		if t, ok := u.thumbs[l.Path]; ok {
			keep[l.Path] = t
		}
	}
	u.thumbs = keep
}

// nextChangeText is the line under the rows.
func nextChangeText(r core.RotationStatusResult, now time.Time) string {
	switch {
	case r.Paused:
		return "Paused. Resume to rotate again."
	case !r.Running:
		return "Not rotating: the service is stopped and no window timer is running."
	case r.State.NextChange == nil:
		return "Next change: not scheduled"
	}
	left := r.State.NextChange.Sub(now).Round(time.Second)
	if left <= 0 {
		// The rotator is running its plugins; the card reloads when it
		// records the change.
		return "Changing now…"
	}
	return fmt.Sprintf("Next change in %d:%02d", int(left.Minutes()), int(left.Seconds())%60)
}

func (u *ui) drawNextChange(now time.Time) {
	if u.nextChange != nil {
		u.nextChange.SetText(nextChangeText(u.rotation, now))
	}
}

/*
wallpaperControls is Pause/Resume and Skip, at the start of the Wallpaper
card's countdown line. Pause shows while nothing is pinned and Resume while anything
is, one button whose label changes, so the line never changes width.
*/
func (u *ui) wallpaperControls() fyne.CanvasObject {
	u.loadRotation()
	r := u.rotation
	label, icon := "Pause", theme.MediaPauseIcon()
	what, op, done := "Pausing…", core.RotationPause, "Paused. The wallpapers on screen stay until you resume."
	if r.Control.Pinned() {
		label, icon = "Resume", theme.MediaPlayIcon()
		what, op, done = "Resuming…", core.RotationResume, "Resumed."
	}
	hold := widget.NewButtonWithIcon(label, icon, func() { u.rotationOp(what, op, done) })
	skip := widget.NewButtonWithIcon("Skip", theme.MediaSkipNextIcon(), func() {
		u.rotationOp("Changing the wallpaper…", core.RotationSkip, "New wallpaper requested.")
	})
	if u.sh.Working() {
		hold.Disable()
		skip.Disable()
	}
	return container.NewHBox(
		// Pause's consequence outlives the window and the service, which the
		// label cannot say; Skip does what it says (spec 013 R6.1).
		widgets.FixedWidth(widgets.WithTip(hold, "Pause pins what is on screen now, through restarts, until Resume clears every pin."), 110),
		skip,
	)
}

func (u *ui) rotationOp(what string, op func(context.Context, core.Request) error, done string) {
	u.sh.Perform(what, func(ctx context.Context) error {
		if err := op(ctx, u.request()); err != nil {
			return err
		}
		fyne.Do(func() {
			u.rotationOK = false
			u.loadRotation()
			u.sh.OK(done)
		})
		return nil
	})
}

// loadRotation reads rotation.json and the pins for the card.
func (u *ui) loadRotation() {
	if u.rotationOK {
		return
	}
	u.rotationOK = true
	u.rotationSeq++
	seq := u.rotationSeq
	run := func() {
		res, err := core.RotationStatus(context.Background(), u.request())
		fyne.Do(func() { u.applyRotation(seq, res, err) })
	}
	if !u.sh.OnScreen() {
		run()
		return
	}
	go run()
}

/*
applyRotation puts a finished load on the card, unless a load was started
after it (spec 029 R3). Loads run off the UI thread and can finish in any
order; without this, a read taken before a change could land after the read
taken after it and put the previous wallpaper back.
*/
func (u *ui) applyRotation(seq int, res core.RotationStatusResult, err error) {
	if seq != u.rotationSeq {
		return
	}
	if err != nil {
		u.sh.Report("Read the wallpaper status", err)
	}
	u.rotation = res
	u.forgetThumbs()
	if u.showsWallpaper() {
		u.sh.Refresh()
	}
}

/*
rotationChanged marks the card's state stale and reloads it while the card is
on screen. Hidden to the tray it only marks it: showWindow reloads a stale
card (spec 029 R2), and a section switch rebuilds it.
*/
func (u *ui) rotationChanged() {
	u.rotationOK = false
	if u.showsWallpaper() && !u.hiddenToTray {
		u.loadRotation()
	}
}

// showsWallpaper reports whether the section on screen has the card.
func (u *ui) showsWallpaper() bool {
	t := u.sh.Current().Title()
	return t == sectionService || t == sectionActivity
}

/*
watchRotation reloads the card whenever rotation.json is replaced, and ticks
the countdown once a second. The rotator writes the file after every change
and every pause, so the card follows the daemon without polling it. The
window's own timer does not rely on this: it reloads the card itself after
each change (recordNext), because the watcher has been seen not to report the
window's own writes on macOS (spec 029).
*/
func (u *ui) watchRotation() func() {
	ctx, cancel := context.WithCancel(context.Background())
	changed, stop, err := core.WatchRotationState(ctx)
	if err != nil {
		paneEvents(u.activity).Warnf("Wallpaper status will not follow changes: %v", err)
	}
	go func() {
		defer stop()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
				fyne.Do(u.rotationChanged)
			case now := <-tick.C:
				fyne.Do(func() {
					if u.showsWallpaper() && !u.hiddenToTray {
						u.drawNextChange(now)
					}
				})
			}
		}
	}()
	return cancel
}

// --- Pin from the review ---------------------------------------------------------

/*
pinButton opens a menu of the targets this desktop has: each monitor, all of
them, and the lock screen where the platform sets one. Choosing one shows the
reviewed image there at once and holds it until Resume.
*/
func (u *ui) pinButton(r *reviewModel) *widget.Button {
	var b *widget.Button
	b = widget.NewButtonWithIcon("Pin", theme.ConfirmIcon(), func() {
		path := r.current()
		if path == "" {
			return
		}
		go func() {
			targets := core.RotationTargets(context.Background(), u.request())
			fyne.Do(func() { u.showPinMenu(b, path, targets) })
		}()
	})
	if r.current() == "" || u.sh.Working() {
		b.Disable()
	}
	return b
}

func (u *ui) showPinMenu(b *widget.Button, path string, t core.RotationTargetsResult) {
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	widget.ShowPopUpMenuAtRelativePosition(fyne.NewMenu("", u.pinMenuItems(path, t)...), c, fyne.NewPos(0, b.Size().Height), b)
}

// pinMenuItems are the Pin menu's targets, as far as the mode sets them: each
// monitor when there is more than one, all of them, and the lock screen.
func (u *ui) pinMenuItems(path string, t core.RotationTargetsResult) []*fyne.MenuItem {
	pinTo := func(label string, monitor int, lock bool) *fyne.MenuItem {
		return fyne.NewMenuItem(label, func() {
			u.sh.Perform("Pinning…", func(ctx context.Context) error {
				err := core.RotationPin(ctx, core.PinRequest{Request: u.request(), Path: path, Monitor: monitor, Lockscreen: lock})
				if err != nil {
					return err
				}
				fyne.Do(func() {
					u.rotationOK = false
					u.sh.OK(fmt.Sprintf("Pinned to %s until you resume in Service.", label))
				})
				return nil
			})
		})
	}
	var items []*fyne.MenuItem
	if t.Monitors > 1 {
		for i := range t.Monitors {
			items = append(items, pinTo(fmt.Sprintf("Monitor %d", i+1), i, false))
		}
	}
	if t.Monitors > 0 {
		items = append(items, pinTo("All monitors", core.AllMonitors, false))
	}
	if t.Lockscreen {
		if len(items) > 0 {
			items = append(items, fyne.NewMenuItemSeparator())
		}
		items = append(items, pinTo("Lock screen", 0, true))
	}
	return items
}
