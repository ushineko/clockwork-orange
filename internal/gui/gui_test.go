package gui

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/events"
	"github.com/ushineko/clockwork-orange/internal/platform"
	"github.com/ushineko/clockwork-orange/internal/plugins"
	"github.com/ushineko/clockwork-orange/internal/store"
)

/*
The window's tests run headless.

Fyne's test driver draws into memory and runs fyne.Do inline, so everything here
works with no DISPLAY and no WAYLAND_DISPLAY -- which is the requirement, since
`make test` runs on machines that have neither.

What is deliberately not tested here is the sections' layout. The tests pin
behaviour instead -- what a form writes, when a button is enabled, what the
review does on Space -- and each one names the bug it prevents (R7.14).
*/

// fakePlatform records what the window asked the desktop to do.
type fakePlatform struct {
	mu   sync.Mutex
	set  []string
	lock []string
}

func (*fakePlatform) Name() string                     { return "fake" }
func (*fakePlatform) MonitorCount(context.Context) int { return 1 }
func (*fakePlatform) LockscreenSupported() bool        { return true }
func (f *fakePlatform) SetWallpaper(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, p)
	return nil
}

func (f *fakePlatform) SetWallpaperMulti(_ context.Context, p []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set = append(f.set, p...)
	return nil
}

func (f *fakePlatform) SetLockscreen(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lock = append(f.lock, p)
	return nil
}

// recordingPlugin stands in for a registry entry and remembers every config
// it was run with; it answers with dir and reports progress like a download.
type recordingPlugin struct {
	name string
	dir  string
	mu   sync.Mutex
	runs []map[string]any
}

func (p *recordingPlugin) Name() string            { return p.name }
func (p *recordingPlugin) Description() string     { return "records its runs" }
func (p *recordingPlugin) Schema() []plugins.Field { return nil }
func (p *recordingPlugin) Run(_ context.Context, cfg map[string]any, ev events.Events) (plugins.Result, error) {
	p.mu.Lock()
	p.runs = append(p.runs, cfg)
	p.mu.Unlock()
	ev.Infof("running %s", p.name)
	ev.Progress(50, "halfway")
	ev.Progress(100, "Done!")
	return plugins.Result{Path: p.dir, Message: "ok"}, nil
}

func (p *recordingPlugin) last() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.runs) == 0 {
		return nil
	}
	return p.runs[len(p.runs)-1]
}

// findButton finds a button by its text anywhere under o; findCheck the
// first check box. Layout containers, scrolls and the fixed-size stacks are
// walked; widgets that hide their children (Form, Card) are not, and the
// tests do not put buttons inside those.
func findButton(o fyne.CanvasObject, text string) *widget.Button {
	var found *widget.Button
	walk(o, func(c fyne.CanvasObject) bool {
		if b, ok := c.(*widget.Button); ok && b.Text == text {
			found = b
			return false
		}
		return true
	})
	return found
}

func findCheck(o fyne.CanvasObject) *widget.Check {
	var found *widget.Check
	walk(o, func(c fyne.CanvasObject) bool {
		if ch, ok := c.(*widget.Check); ok {
			found = ch
			return false
		}
		return true
	})
	return found
}

func walk(o fyne.CanvasObject, visit func(fyne.CanvasObject) bool) bool {
	if !visit(o) {
		return false
	}
	switch c := o.(type) {
	case *fyne.Container:
		for _, child := range c.Objects {
			if !walk(child, visit) {
				return false
			}
		}
	case *container.Scroll:
		return walk(c.Content, visit)
	}
	return true
}

/*
testUI is a window with no window: enough of a *ui for the parts that do not
draw, over temp databases, a fake desktop and a fake systemctl, with HOME
redirected so nothing touches the developer's real config. Sections built
from it load inline (onScreen is false), so a test sees finished state when
the builder returns.
*/
func testUI(t *testing.T) (*ui, *fakePlatform, *platform.FakeRunner) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLOCKWORK_LOCK_DIR", home)
	app := test.NewApp()
	t.Cleanup(app.Quit)

	hist, err := store.OpenHistory(filepath.Join(home, "history.db"))
	require.NoError(t, err)
	bl, err := store.OpenBlacklist(filepath.Join(home, "blacklist.db"))
	require.NoError(t, err)
	fp := &fakePlatform{}
	runner := &platform.FakeRunner{Stdout: "inactive\n"}
	deps := &core.Deps{Platform: fp, Service: platform.NewService(runner), History: hist, Blacklist: bl}
	t.Cleanup(func() { _ = deps.Close() })

	u := &ui{
		app:      app,
		win:      test.NewWindow(widget.NewLabel("")),
		flashes:  container.NewVBox(),
		deps:     deps,
		version:  "vtest",
		activity: newLogPane(),
		scheme:   palettes[0].name,
	}
	t.Cleanup(func() { u.shutdown(); u.win.Close() })
	u.loadConfigNow()
	u.plugins = core.AvailablePlugins()
	return u, fp, runner
}

func writeDoc(t *testing.T, doc config.Document) string {
	t.Helper()
	path := config.DefaultPath()
	require.NoError(t, config.Save(path, doc))
	return path
}

func imageDir(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range n {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "img"+string(rune('a'+i))+".jpg"), []byte("x"), 0o600))
	}
	return dir
}

// --- names and the registry ------------------------------------------------------

// The flag help lists these while parsing flags, before there is a Fyne app to
// construct a theme icon against, and every advertised name must have a
// section behind it (R7.3).
func TestSectionNamesNeedNoAppAndEveryOneHasABuilder(t *testing.T) {
	names := SectionNames()
	require.Equal(t, firstSectionTitle(), names[0])
	require.Equal(t, []string{"Local", "Wallhaven", "Duckduckgo Images"}, names[1:4],
		"one section per registered plugin, `_` → space, Title Case")
	require.Equal(t, []string{"History", "Blacklist", "Settings", "Appearance", "About"}, names[4:])
	for _, n := range names {
		require.Containsf(t, sectionBuilders(), n, "%q is advertised but has no section", n)
	}
	// Service and Activity both exist as builders; only one is advertised.
	require.Len(t, sectionBuilders(), len(names)+1)
}

func TestSchemeNamesListsEveryPalette(t *testing.T) {
	require.Equal(t, []string{"Breeze Dark", "Breeze Light", "Oxygen Dark", "Adwaita Dark", "Adwaita Light"}, SchemeNames())
	require.Equal(t, "Breeze Dark", paletteByName("no such scheme").name, "a stale preference must fall back rather than fail")
}

// --section opens the named section, case-insensitively; a typo opens the
// first rather than a dead window.
func TestSectionIndexResolvesNamesAndFallsBackToTheFirst(t *testing.T) {
	_, _, _ = testUI(t)
	secs := sections()
	require.Equal(t, 0, sectionIndex(secs, ""))
	require.Equal(t, 0, sectionIndex(secs, "nope"))
	require.Equal(t, len(secs)-1, sectionIndex(secs, "about"))
	require.Equal(t, 2, sectionIndex(secs, "Wallhaven"))
}

// A name in Actions() is a claim that the GUI reaches that operation. A
// duplicate would make the parity test pass with one of them unimplemented.
func TestActionsAreUniqueAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Actions() {
		require.NotEmpty(t, a)
		require.Falsef(t, seen[a], "%q is listed twice", a)
		seen[a] = true
	}
	require.NotEmpty(t, seen)
}

// Every section renders headlessly with an empty document and with a
// populated one (R7.14). A builder that panics on a missing key would take the
// window down on first launch.
func TestEverySectionRendersHeadlessly(t *testing.T) {
	u, _, _ := testUI(t)
	for _, s := range sections() {
		require.NotPanicsf(t, func() { _ = s.build(u) }, "empty document: %s", s.title)
	}
	for title, b := range sectionBuilders() {
		require.NotPanicsf(t, func() { _ = b.build(u) }, "empty document: %s", title)
	}

	dir := imageDir(t, 2)
	doc := config.Defaults()
	doc.DualWallpapers = true
	doc.Plugins["local"] = map[string]any{"enabled": true, "path": dir}
	doc.Plugins["wallhaven"] = map[string]any{"enabled": true, "query": "landscape, forest", "category_general": true}
	doc.Plugins["stable_diffusion"] = map[string]any{"enabled": false, "prompt": "castle"}
	writeDoc(t, doc)
	u.loadConfigNow()
	require.NoError(t, u.deps.Blacklist.Add("abc", "local", filepath.Join(dir, "imga.jpg")))
	for _, s := range sections() {
		require.NotPanicsf(t, func() { _ = s.build(u) }, "populated document: %s", s.title)
	}
}

// --- flash (copied from nmsbonker) ----------------------------------------------

func TestFlashKeepsFailuresUntilDismissed(t *testing.T) {
	_, fades := flashHold(StatusBad)
	require.False(t, fades, "a failure must not clear itself")
	warn, fades := flashHold(StatusWarn)
	require.True(t, fades)
	good, _ := flashHold(StatusGood)
	require.Greater(t, warn, good, "a warning names a condition to act on, so it stays longer")
	require.GreaterOrEqual(t, good.Seconds(), 5.0)
}

func TestFlashShowsOneBannerAtATime(t *testing.T) {
	u, _, _ := testUI(t)
	u.flash("first", StatusGood)
	require.Len(t, u.flashes.Objects, 1)
	u.flash("second", StatusBad)
	require.Len(t, u.flashes.Objects, 1, "a newer result replaces the older one")
	stale := u.flashSeq - 1
	u.clearFlash(stale)
	require.Len(t, u.flashes.Objects, 1, "a stale fade timer must not clear the newer banner")
	u.clearFlash(u.flashSeq)
	require.Empty(t, u.flashes.Objects)
}

func TestBrowseButtonOpensAChooserWithoutPanicking(t *testing.T) {
	u, _, _ := testUI(t)
	for _, dir := range []bool{false, true} {
		field := widget.NewEntry()
		require.NotPanics(t, func() { test.Tap(u.browseButton(field, dir)) })
	}
	require.NotPanics(t, func() { u.chooseFolder("", func(string) {}) })
	for _, text := range []string{"", "/nonexistent/path/for/a/test", "~"} {
		require.NotNil(t, pickerStart(text))
	}
}

// --- window geometry (R7.2) --------------------------------------------------------

// The window opens at the size the config remembers, and at the default when
// the file has none; a resize is written back through the auto-save.
func TestWindowSizeRestoresFromTheConfigAndPersistsAfterAResize(t *testing.T) {
	u, _, _ := testUI(t)
	require.Equal(t, fyne.NewSize(defaultWindowWidth, defaultWindowHeight), windowSize(u.doc, false),
		"no file: the default, not the Defaults() 800×600 the Python restored")
	doc := config.Defaults()
	doc.WindowWidth, doc.WindowHeight = 1024, 700
	writeDoc(t, doc)
	u.loadConfigNow()
	require.Equal(t, fyne.NewSize(1024, 700), windowSize(u.doc, u.docExists))

	saved := make(chan struct{}, 4)
	u.saved = func() { saved <- struct{}{} }
	u.noteSize(fyne.NewSize(1300, 800))
	u.noteSize(fyne.NewSize(1300, 800)) // the poll sees the same size again: no second save
	<-saved
	got, err := config.Load(config.DefaultPath())
	require.NoError(t, err)
	require.Equal(t, 1300, got.WindowWidth)
	require.Equal(t, 800, got.WindowHeight)
	select {
	case <-saved:
		t.Fatal("an unchanged size must not write again")
	default:
	}
}
