// Copied from nmsbonker (same author) — keep in sync by hand, minus luaFilter,
// chooseFile, confirmWithBody, pathDialog and showDetail, which this project has
// no use for; openPath opens a
// directory here rather than a script.

package gui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/ushineko/clockwork-orange/internal/config"
)

// confirmDestructive is a confirmation that names what is about to happen in at
// least the detail the CLI gives, with the destructive button styled as
// destructive and the cancel as the safe default.
//
// The detail says what is *not* touched as well as what is (R6). Every
// destructive path in this window leaves something alone that a user might
// reasonably fear for — the images on disk, the history that stops re-downloads, the config in
// the library — and not saying so is how a confirmation dialog becomes the
// thing people click through without reading.
func (u *ui) confirmDestructive(title, detail, confirm string, do func()) {
	body := widget.NewLabel(detail)
	body.Wrapping = fyne.TextWrapWord

	d := dialog.NewCustomWithoutButtons(title, container.NewVBox(body), u.win)

	cancel := widget.NewButton("Cancel", func() { d.Hide() })
	proceed := widget.NewButton(confirm, func() {
		d.Hide()
		do()
	})
	proceed.Importance = widget.DangerImportance

	d.SetButtons([]fyne.CanvasObject{cancel, proceed})
	d.Resize(fyne.NewSize(560, 320))
	d.Show()
}

// fixedHeight and fixedWidth pin a widget's minimum size. Fyne's list and table
// take all the space they are given; these keep a section's layout stable while
// it is being looked at, which is what stops the build log from growing the
// window a line at a time.
func fixedHeight(o fyne.CanvasObject, h float32) fyne.CanvasObject {
	pad := canvas.NewRectangle(nil)
	pad.SetMinSize(fyne.NewSize(0, h))
	return container.New(layout.NewStackLayout(), pad, o)
}

func fixedWidth(o fyne.CanvasObject, w float32) fyne.CanvasObject {
	pad := canvas.NewRectangle(nil)
	pad.SetMinSize(fyne.NewSize(w, 0))
	return container.New(layout.NewStackLayout(), pad, o)
}

// --- file chooser ----------------------------------------------------------

// pickerStart is where a chooser should open: the path already in the field if
// it names a directory, otherwise its parent, otherwise the home directory.
// A field left empty, or holding something that no longer exists, must not
// leave the chooser at whatever directory the process happens to be in.
func pickerStart(text string) fyne.ListableURI {
	candidates := []string{}
	if p := config.ExpandPath(text); p != "" {
		candidates = append(candidates, p, filepath.Dir(p))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, home)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err != nil || !fi.IsDir() {
			continue
		}
		if lu, err := storage.ListerForURI(storage.NewFileURI(c)); err == nil {
			return lu
		}
	}
	return nil
}

// browseButton is the affordance beside a path field: it opens the platform
// chooser and writes the chosen path back into the field. The field stays
// editable — a path can still be typed or pasted, which is the only way to
// reach somewhere the chooser will not show.
//
// `dir` picks a directory chooser rather than a file one.
func (u *ui) browseButton(field *widget.Entry, dir bool) *widget.Button {
	choose := func() {
		if dir {
			d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
				if err != nil || lu == nil {
					return
				}
				field.SetText(lu.Path())
			}, u.win)
			d.SetLocation(pickerStart(field.Text))
			// Resize only after Show. Before Show the dialog has no window,
			// and Resize asks it for its minimum size — which in fyne 2.8.1
			// dereferences that nil window and takes the process with it.
			d.Show()
			d.Resize(fyne.NewSize(760, 520))
			return
		}
		d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil || rc == nil {
				return
			}
			// The chooser hands back an open handle; core reads the file
			// itself, by path, so close it immediately rather than holding a
			// descriptor open for the life of the dialog.
			path := rc.URI().Path()
			_ = rc.Close()
			field.SetText(path)
		}, u.win)
		d.SetLocation(pickerStart(field.Text))
		d.Show()
		d.Resize(fyne.NewSize(760, 520))
	}
	return widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), choose)
}

// withBrowse lays a path field out with its chooser button on the right.
func (u *ui) withBrowse(field *widget.Entry, dir bool) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, u.browseButton(field, dir), field)
}

// chooseFolder opens a directory chooser.
func (u *ui) chooseFolder(start string, then func(path string)) {
	d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
		if err != nil || lu == nil {
			return
		}
		then(lu.Path())
	}, u.win)
	d.SetLocation(pickerStart(start))
	d.Show()
	d.Resize(fyne.NewSize(760, 520))
}

/*
openPath hands a file or directory to the desktop.

This is the one place the window starts a process of its own, and it is
deliberately the smallest possible one: the desktop's opener decides what shows
a directory of wallpapers, because the desktop already knows and this program
has no business having an opinion (plugins_tab.py open_file_manager). A machine
without an opener — a bare window manager, a container — gets a warning banner
naming the path, which is still enough to open it by hand.
*/
func (u *ui) openPath(path string) {
	if path == "" {
		u.flash("There is nothing to open yet.", StatusWarn)
		return
	}
	go func() {
		done := u.busy("Opening " + filepath.Base(path) + "…")
		defer done()
		// The path is one this program produced or read out of its own settings
		// -- a script in the library, the workspace, the report directory --
		// and it is handed to xdg-open as a single argv element, so no shell
		// parses it.
		//
		// context.Background, deliberately: the point of this call is to hand
		// the file to whatever the desktop opens it with and let go. Tying the
		// child to a context of ours would mean closing the window took the
		// user's text editor with it.
		cmd := exec.CommandContext(context.Background(), openerCommand(), path) //nolint:gosec // a path from this program's own settings, passed as one argv element
		if err := cmd.Start(); err != nil {
			fyne.Do(func() {
				u.flash(fmt.Sprintf("Could not ask the desktop to open %s: %v. "+
					"Open it by hand; nothing else was affected.", path, err), StatusWarn)
			})
		}
	}()
}

// openerCommand is the desktop's "open this" program: xdg-open on Linux,
// open on macOS, explorer on Windows.
func openerCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "open"
	case "windows":
		return "explorer"
	default:
		return "xdg-open"
	}
}
