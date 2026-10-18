package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/engine"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/imaging"
	"github.com/ushineko/clockwork-orange/internal/platform"
)

/*
Rotation state and control (spec 026).

Two JSON files in the state directory carry what the rotator is showing and
what the user asked of it, because the daemon and the window share nothing
else but the cycling lock and the config file.

  - rotation.json is what is on screen, when it changes next and who rotates.
    The rotator writes it after every cycle; RotationPin writes it too, since
    a pin puts an image on screen without waiting for the rotator.
  - rotation-control.json is the pins and the skip counter. The front ends
    write it; the rotator writes it only to drop a pin whose file is gone
    (R9) or to move a pin to the image a skip chose.

Every read-modify-write holds the rotation lock, so a CLI and a window
writing at once cannot lose each other's change, and every write replaces
the file by rename, so a reader never sees half of one.
*/

// Rotators: who wrote rotation.json.
const (
	RotatorDaemon = "daemon"
	RotatorGUI    = "gui"
	RotatorCLI    = "cli"
)

const (
	rotationFile        = "rotation.json"
	rotationControlFile = "rotation-control.json"
	rotationLockID      = "clockwork_orange_rotation_lock"
	// rotationLockWait bounds how long a writer waits for another one.
	rotationLockWait = 5 * time.Second
)

// ErrNothingShown is returned by RotationPause before any rotation recorded
// what is on screen.
var ErrNothingShown = errors.New("no wallpaper has been recorded yet; wait for the first change or use Skip")

// RotationPath is where rotation.json lives.
func RotationPath() string { return filepath.Join(config.StateDir(), rotationFile) }

// RotationControlPath is where rotation-control.json lives.
func RotationControlPath() string { return filepath.Join(config.StateDir(), rotationControlFile) }

// Shown is one image on screen.
type Shown struct {
	Path  string    `json:"path"`
	Since time.Time `json:"since"`
}

// RotationState is rotation.json.
type RotationState struct {
	Rotator string `json:"rotator,omitempty"`
	Mode    string `json:"mode,omitempty"`
	// Desktop is one entry per monitor, in the platform's desktop order.
	Desktop    []Shown `json:"desktop,omitempty"`
	Lockscreen *Shown  `json:"lockscreen,omitempty"`
	// NextChange is empty while every target is pinned or nothing rotates.
	NextChange *time.Time `json:"next_change,omitempty"`
	Updated    time.Time  `json:"updated"`
}

// RotationControl is rotation-control.json.
type RotationControl struct {
	// Desktop maps a monitor index to its pinned image.
	Desktop    map[int]string `json:"desktop,omitempty"`
	Lockscreen string         `json:"lockscreen,omitempty"`
	// Skip is incremented by every skip request; a rotator acts when it
	// differs from the value it last saw.
	Skip int `json:"skip"`
}

// Pinned reports whether anything is pinned.
func (c RotationControl) Pinned() bool { return len(c.Desktop) > 0 || c.Lockscreen != "" }

// paused reports whether every target mode sets on n monitors is pinned, so
// a rotation would change nothing.
func (c RotationControl) paused(mode Mode, n int) bool {
	if setsDesktop(mode) {
		for i := range n {
			if c.Desktop[i] == "" {
				return false
			}
		}
	}
	if setsLockscreen(mode) && c.Lockscreen == "" {
		return false
	}
	return true
}

func setsDesktop(m Mode) bool    { return m != ModeLockscreen }
func setsLockscreen(m Mode) bool { return m == ModeLockscreen || m == ModeDual }

func parseMode(s string) Mode {
	for _, m := range []Mode{ModeDefault, ModeDesktop, ModeLockscreen, ModeDual} {
		if m.String() == s {
			return m
		}
	}
	return ModeDefault
}

// --- files ------------------------------------------------------------------

// rotationMu serialises writers inside one process; the platform lock
// serialises processes.
var rotationMu sync.Mutex

func withRotationLock(fn func() error) error {
	rotationMu.Lock()
	defer rotationMu.Unlock()
	deadline := time.Now().Add(rotationLockWait)
	for {
		if lock, ok := platform.TryLock(rotationLockID); ok {
			defer lock.Release()
			return fn()
		}
		if time.Now().After(deadline) {
			return errors.New("another clockwork-orange process is holding the rotation files")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) //nolint:gosec // G304: a fixed file in this program's own state directory.
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := config.MkdirAll(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// LoadRotation reads both files. A missing file is the zero value.
func LoadRotation() (RotationState, RotationControl, error) {
	var st RotationState
	var ctl RotationControl
	return st, ctl, errors.Join(readJSON(RotationPath(), &st), readJSON(RotationControlPath(), &ctl))
}

// updateRotation runs fn over both files under the lock and writes back the
// ones it reports changed.
func updateRotation(fn func(st *RotationState, ctl *RotationControl) (stChanged, ctlChanged bool, err error)) error {
	return withRotationLock(func() error {
		st, ctl, err := LoadRotation()
		if err != nil {
			return err
		}
		stChanged, ctlChanged, err := fn(&st, &ctl)
		if err != nil {
			return err
		}
		if stChanged {
			st.Updated = time.Now()
			if err := writeJSON(RotationPath(), st); err != nil {
				return err
			}
		}
		if ctlChanged {
			return writeJSON(RotationControlPath(), ctl)
		}
		return nil
	})
}

// updateControl is updateRotation for a front end that only changes pins.
func updateControl(fn func(ctl *RotationControl)) error {
	return updateRotation(func(_ *RotationState, ctl *RotationControl) (bool, bool, error) {
		fn(ctl)
		return false, true, nil
	})
}

// --- status -----------------------------------------------------------------

// RotationStatusResult is what the Service section and `wallpaper status`
// show.
type RotationStatusResult struct {
	State   RotationState
	Control RotationControl
	// Running is true while the rotator named in State is alive.
	Running bool
	// Paused is true when every target the mode sets is pinned.
	Paused bool
}

// RotationStatus reads the state and checks whether its rotator is alive:
// the daemon by its cycling lock, the window by its single-instance lock (and
// the daemon not holding the cycling lock, which idles the window's timer).
func RotationStatus(_ context.Context, _ Request) (RotationStatusResult, error) {
	st, ctl, err := LoadRotation()
	res := RotationStatusResult{State: st, Control: ctl}
	daemon := DaemonRunning()
	switch st.Rotator {
	case RotatorDaemon:
		res.Running = daemon
	case RotatorGUI:
		res.Running = !daemon && GUIRunning()
	}
	res.Paused = ctl.Pinned() && ctl.paused(parseMode(st.Mode), len(st.Desktop))
	return res, err
}

// GUIRunning reports whether a window holds the GUI single-instance lock.
func GUIRunning() bool {
	lock, ok := platform.TryLock(guiLockID)
	if ok {
		lock.Release()
	}
	return !ok
}

// --- pause, resume, skip ------------------------------------------------------

// RotationPause pins every image the mode shows, as rotation.json records it.
func RotationPause(_ context.Context, _ Request) error {
	return updateRotation(func(st *RotationState, ctl *RotationControl) (bool, bool, error) {
		mode := parseMode(st.Mode)
		if (setsDesktop(mode) && len(st.Desktop) == 0) || (setsLockscreen(mode) && st.Lockscreen == nil) {
			return false, false, ErrNothingShown
		}
		if setsDesktop(mode) {
			ctl.Desktop = map[int]string{}
			for i, s := range st.Desktop {
				ctl.Desktop[i] = s.Path
			}
		}
		if setsLockscreen(mode) {
			ctl.Lockscreen = st.Lockscreen.Path
		}
		st.NextChange = nil
		return true, true, nil
	})
}

// RotationResume clears every pin.
func RotationResume(_ context.Context, _ Request) error {
	return updateControl(func(ctl *RotationControl) {
		ctl.Desktop = nil
		ctl.Lockscreen = ""
	})
}

// RotationSkip asks the running rotator for a new selection now. With no
// rotator running, the change happens in this process instead, so a skip
// always does something.
func RotationSkip(ctx context.Context, req Request) error {
	status, err := RotationStatus(ctx, req)
	if err != nil {
		return err
	}
	if status.Running {
		return updateControl(func(ctl *RotationControl) { ctl.Skip++ })
	}
	doc, err := LoadConfig(ctx, req)
	if err != nil {
		return err
	}
	_, err = Rotate(ctx, RotateRequest{
		CycleRequest: CycleRequest{Request: req, Mode: ResolveMode(doc.Doc, false, false)},
		Rotator:      RotatorCLI,
		Skip:         true,
	})
	return err
}

// --- pin --------------------------------------------------------------------

// PinRequest pins one image.
type PinRequest struct {
	Request
	Path string
	// Monitor is the desktop index, or AllMonitors.
	Monitor int
	// Lockscreen pins the lock screen instead of a desktop.
	Lockscreen bool
}

// AllMonitors in PinRequest.Monitor pins every desktop.
const AllMonitors = -1

// ValidatePinPath requires an absolute path to an existing regular image file
// (R10).
func ValidatePinPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("not an absolute path: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot pin %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	if !imaging.IsImageFile(path) {
		return fmt.Errorf("not a supported image: %s", path)
	}
	return nil
}

/*
RotationPin puts the image on its target at once and records the pin, so it
works with nothing rotating and holds when rotation starts (R5).

A platform that sets one screen (platform.ScreenSetter, spec 028) touches only
the pinned one. Elsewhere every monitor is set: the others keep the images
rotation.json says they have, and a monitor with no record gets the pinned
image too.
*/
func RotationPin(ctx context.Context, req PinRequest) error {
	if err := ValidatePinPath(req.Path); err != nil {
		return err
	}
	d := req.deps()
	d.ensurePlatform(req.Events)
	p := d.Platform
	if req.Lockscreen {
		if !p.LockscreenSupported() {
			return errors.New("this platform has no lock screen wallpaper to pin")
		}
	}
	n := max(p.MonitorCount(ctx), 1)
	if !req.Lockscreen && req.Monitor != AllMonitors && (req.Monitor < 0 || req.Monitor >= n) {
		return fmt.Errorf("monitor %d does not exist; this desktop has %d", req.Monitor+1, n)
	}
	now := time.Now()
	return updateRotation(func(st *RotationState, ctl *RotationControl) (bool, bool, error) {
		if req.Lockscreen {
			if err := p.SetLockscreen(ctx, req.Path); err != nil {
				return false, false, err
			}
			ctl.Lockscreen = req.Path
			st.Lockscreen = &Shown{Path: req.Path, Since: now}
			return true, true, nil
		}
		images := make([]string, n)
		for i := range n {
			switch {
			case req.Monitor == AllMonitors || i == req.Monitor:
				images[i] = req.Path
			case i < len(st.Desktop) && regularFile(st.Desktop[i].Path):
				images[i] = st.Desktop[i].Path
			default:
				images[i] = req.Path
			}
		}
		var err error
		one, oneScreen := p.(platform.ScreenSetter)
		switch {
		case req.Monitor == AllMonitors || n == 1:
			err = p.SetWallpaper(ctx, req.Path)
		case oneScreen:
			err = one.SetWallpaperOn(ctx, req.Monitor, req.Path)
			// What the others show, for the record, from the desktop
			// itself where the record has nothing.
			if r, ok := p.(platform.WallpaperReader); ok {
				for i := range n {
					if i != req.Monitor && (i >= len(st.Desktop) || !regularFile(st.Desktop[i].Path)) {
						if path, rerr := r.WallpaperOf(ctx, i); rerr == nil && path != "" {
							images[i] = path
						}
					}
				}
			}
		default:
			err = p.SetWallpaperMulti(ctx, images)
		}
		if err != nil {
			return false, false, err
		}
		if ctl.Desktop == nil {
			ctl.Desktop = map[int]string{}
		}
		for i := range n {
			if req.Monitor == AllMonitors || i == req.Monitor {
				ctl.Desktop[i] = req.Path
			}
		}
		st.Desktop = recordShown(st.Desktop, images, now)
		return true, true, nil
	})
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// recordShown is images as Shown entries, keeping the time of an image that
// did not change.
func recordShown(prev []Shown, images []string, now time.Time) []Shown {
	out := make([]Shown, len(images))
	for i, img := range images {
		out[i] = Shown{Path: img, Since: now}
		if i < len(prev) && prev[i].Path == img && !prev[i].Since.IsZero() {
			out[i].Since = prev[i].Since
		}
	}
	return out
}

// --- the rotator's step -------------------------------------------------------

// RotateRequest is one rotation by a rotator.
type RotateRequest struct {
	CycleRequest
	// Rotator is recorded in rotation.json.
	Rotator string
	// Skip asks for new images now. On a paused display (every target
	// pinned) it replaces the pinned images too and moves the pins to them;
	// otherwise pinned targets keep their images and the rest change.
	Skip bool
}

// Rotate is Cycle with pins honoured and the outcome recorded (R1, R3, R4).
// With nothing pinned and no skip it selects exactly as Cycle does; with every
// target pinned and no skip it changes nothing.
func Rotate(ctx context.Context, req RotateRequest) (CycleResult, error) {
	d := req.deps()
	d.ensurePlatform(req.Events)
	n := max(d.Platform.MonitorCount(ctx), 1)

	_, ctl, err := LoadRotation()
	if err != nil {
		req.Events.Warnf("Rotation files unreadable, rotating without pins: %v", err)
	}
	if dropped := dropStalePins(&ctl, n); len(dropped) > 0 {
		for _, msg := range dropped {
			req.Events.Infof("%s", msg)
		}
		if err := updateControl(func(c *RotationControl) { dropStalePins(c, n) }); err != nil {
			req.Events.Warnf("Could not update rotation-control.json: %v", err)
		}
	}
	paused := ctl.Pinned() && ctl.paused(req.Mode, n)
	if paused && !req.Skip {
		return CycleResult{}, nil
	}
	repin := paused && req.Skip

	res, err := cycleSources(ctx, req.CycleRequest)
	if err != nil {
		return res, err
	}
	rec := &recordingSetter{Setter: d.Platform, n: n}
	if ctl.Pinned() {
		st, _, _ := LoadRotation()
		err = setPinned(ctx, rec, req.Mode, res.Sources, n, &ctl, repin, st, req.Events)
	} else {
		err = setFromSources(ctx, rec, req.Mode, res.Sources, req.Events)
	}
	if rec.desktop == nil && rec.lock == "" {
		return res, err
	}
	recErr := recordRotation(req, rec, ctl, repin)
	return res, errors.Join(err, recErr)
}

// recordRotation writes what a rotation put on screen into rotation.json and,
// after a skip on a paused display, moves the pins to the new images.
func recordRotation(req RotateRequest, rec *recordingSetter, ctl RotationControl, repin bool) error {
	return updateRotation(func(st *RotationState, c *RotationControl) (bool, bool, error) {
		now := time.Now()
		st.Rotator = req.Rotator
		st.Mode = req.Mode.String()
		if rec.desktop != nil {
			st.Desktop = recordShown(st.Desktop, rec.desktop, now)
		}
		if rec.lock != "" {
			prev := st.Lockscreen
			st.Lockscreen = &Shown{Path: rec.lock, Since: now}
			if prev != nil && prev.Path == rec.lock {
				st.Lockscreen.Since = prev.Since
			}
		}
		if !repin {
			return true, false, nil
		}
		// A skip on a paused display moved the pins to the new images.
		changed := false
		for i, p := range ctl.Desktop {
			if c.Desktop[i] != "" && c.Desktop[i] != p {
				c.Desktop[i] = p
				changed = true
			}
		}
		if c.Lockscreen != "" && ctl.Lockscreen != "" && c.Lockscreen != ctl.Lockscreen {
			c.Lockscreen = ctl.Lockscreen
			changed = true
		}
		return true, changed, nil
	})
}

// dropStalePins removes pins whose file is gone or whose monitor no longer
// exists (R9), and says what it dropped.
func dropStalePins(ctl *RotationControl, n int) []string {
	var dropped []string
	for i, p := range ctl.Desktop {
		switch {
		case i >= n:
			dropped = append(dropped, fmt.Sprintf("Monitor %d is gone; its pin is dropped", i+1))
			delete(ctl.Desktop, i)
		case !regularFile(p):
			dropped = append(dropped, fmt.Sprintf("Pinned image is gone, monitor %d rotates again: %s", i+1, p))
			delete(ctl.Desktop, i)
		}
	}
	if ctl.Lockscreen != "" && !regularFile(ctl.Lockscreen) {
		dropped = append(dropped, "Pinned lock screen image is gone, it rotates again: "+ctl.Lockscreen)
		ctl.Lockscreen = ""
	}
	return dropped
}

/*
setPinned selects around the pins. A pinned monitor keeps its image, or with
repin draws a new one and ctl's pin moves to it; an unpinned one draws while
avoiding every image already chosen or pinned. A draw that fails keeps the
monitor's previous image when there is one. The desktop is set only when a
monitor changes; the lock screen likewise.
*/
func setPinned(ctx context.Context, s engine.Setter, mode Mode, sources []string, n int, ctl *RotationControl, repin bool, prev RotationState, ev events.Events) error {
	var errs []error
	images := make([]string, n)
	if setsDesktop(mode) {
		avoid := make([]string, 0, n+len(ctl.Desktop))
		for _, p := range ctl.Desktop {
			avoid = append(avoid, p)
		}
		changed := false
		for i := range n {
			pin := ctl.Desktop[i]
			if pin != "" && !repin {
				images[i] = pin
				continue
			}
			img, err := engine.PickAvoiding(sources, avoid, ev)
			if err != nil {
				if i >= len(prev.Desktop) {
					return err
				}
				img = prev.Desktop[i].Path
			}
			images[i] = img
			avoid = append(avoid, img)
			if pin != "" {
				ctl.Desktop[i] = img
			}
			changed = changed || i >= len(prev.Desktop) || prev.Desktop[i].Path != img
		}
		if changed {
			var err error
			if n > 1 {
				err = s.SetWallpaperMulti(ctx, images)
			} else {
				err = s.SetWallpaper(ctx, images[0])
			}
			errs = append(errs, err)
		}
	}
	if setsLockscreen(mode) && (ctl.Lockscreen == "" || repin) {
		var avoid []string
		if setsDesktop(mode) {
			avoid = images
		}
		img, err := engine.PickLockscreenDistinct(sources, avoid, ev)
		if err != nil {
			return errors.Join(append(errs, err)...)
		}
		if ctl.Lockscreen != "" {
			ctl.Lockscreen = img
		}
		errs = append(errs, s.SetLockscreen(ctx, img))
	}
	return errors.Join(errs...)
}

// recordingSetter passes calls through and remembers what succeeded, as one
// image per monitor.
type recordingSetter struct {
	engine.Setter
	n       int
	desktop []string
	lock    string
}

func (r *recordingSetter) MonitorCount(context.Context) int { return r.n }

func (r *recordingSetter) SetWallpaper(ctx context.Context, path string) error {
	if err := r.Setter.SetWallpaper(ctx, path); err != nil {
		return err
	}
	r.desktop = make([]string, r.n)
	for i := range r.desktop {
		r.desktop[i] = path
	}
	return nil
}

// SetWallpaperMulti records images[i % len] for monitor i, the rule every
// platform's multi-set follows.
func (r *recordingSetter) SetWallpaperMulti(ctx context.Context, paths []string) error {
	if err := r.Setter.SetWallpaperMulti(ctx, paths); err != nil {
		return err
	}
	r.desktop = make([]string, r.n)
	for i := range r.desktop {
		r.desktop[i] = paths[i%len(paths)]
	}
	return nil
}

func (r *recordingSetter) SetLockscreen(ctx context.Context, path string) error {
	if err := r.Setter.SetLockscreen(ctx, path); err != nil {
		return err
	}
	r.lock = path
	return nil
}

// --- scheduling, shared by the daemon and the window ---------------------------

// SetNextChange records when the rotator changes next; nil means paused.
func SetNextChange(rotator string, mode Mode, next *time.Time) error {
	return updateRotation(func(st *RotationState, _ *RotationControl) (bool, bool, error) {
		st.Rotator = rotator
		st.Mode = mode.String()
		st.NextChange = next
		return true, false, nil
	})
}

// RotationPaused reports whether every target mode sets is pinned, over the
// platform's monitors.
func RotationPaused(ctx context.Context, req Request, mode Mode) bool {
	d := req.deps()
	d.ensurePlatform(req.Events)
	_, ctl, _ := LoadRotation()
	return ctl.Pinned() && ctl.paused(mode, max(d.Platform.MonitorCount(ctx), 1))
}

// WatchRotationControl reports changes to rotation-control.json.
func WatchRotationControl(ctx context.Context) (<-chan struct{}, func(), error) {
	return watchFile(ctx, RotationControlPath())
}

// WatchRotationState reports changes to rotation.json.
func WatchRotationState(ctx context.Context) (<-chan struct{}, func(), error) {
	return watchFile(ctx, RotationPath())
}

func watchFile(ctx context.Context, path string) (<-chan struct{}, func(), error) {
	w, err := config.Watch(ctx, path)
	if err != nil {
		return nil, func() {}, err
	}
	return w.Changed(), func() { _ = w.Close() }, nil
}

// RotationTargetsResult is what a pin can name on this desktop.
type RotationTargetsResult struct {
	// Monitors is zero when the mode sets no desktop.
	Monitors   int
	Lockscreen bool
}

// RotationTargets reports the targets the mode in the config sets and the
// platform has, for the Review tab's Pin menu (R8).
func RotationTargets(ctx context.Context, req Request) RotationTargetsResult {
	d := req.deps()
	d.ensurePlatform(req.Events)
	mode := ModeDefault
	if loaded, err := LoadConfig(ctx, req); err == nil {
		mode = ResolveMode(loaded.Doc, false, false)
	}
	var res RotationTargetsResult
	if setsDesktop(mode) {
		res.Monitors = max(d.Platform.MonitorCount(ctx), 1)
	}
	res.Lockscreen = setsLockscreen(mode) && d.Platform.LockscreenSupported()
	return res
}

// --- the desktop's own changes (spec 028 R4) -----------------------------------

// WatchDesktop reports the image each monitor shows as it changes, on a
// platform that can say (platform.WallpaperWatcher); nil elsewhere, which a
// select never receives from.
func WatchDesktop(ctx context.Context, req Request) <-chan platform.ScreenImage {
	d := req.deps()
	d.ensurePlatform(req.Events)
	w, ok := d.Platform.(platform.WallpaperWatcher)
	if !ok {
		return nil
	}
	ch, err := w.WatchWallpapers(ctx)
	if err != nil {
		req.Events.Warnf("Wallpaper changes made outside clockwork-orange will not show: %v", err)
		return nil
	}
	return ch
}

/*
NoteDesktopWallpaper records what the desktop says a monitor shows. The
rotator calls it between its own changes, so an image it set itself is
already in rotation.json and nothing happens. Anything else was set outside
this program: it is recorded, and a pin on that monitor is dropped, because
the user chose something else.
*/
func NoteDesktopWallpaper(img platform.ScreenImage, ev events.Events) error {
	return updateRotation(func(st *RotationState, ctl *RotationControl) (bool, bool, error) {
		if img.Screen < 0 || img.Screen >= len(st.Desktop) || st.Desktop[img.Screen].Path == img.Path {
			return false, false, nil
		}
		st.Desktop[img.Screen] = Shown{Path: img.Path, Since: time.Now()}
		ev.Infof("Monitor %d was changed outside clockwork-orange: %s", img.Screen+1, img.Path)
		pin := ctl.Desktop[img.Screen]
		if pin == "" || pin == img.Path {
			return true, false, nil
		}
		delete(ctl.Desktop, img.Screen)
		ev.Infof("Monitor %d is no longer pinned", img.Screen+1)
		return true, true, nil
	})
}
