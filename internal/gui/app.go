/*
Package gui is the desktop front end of spec 010 (R7).

The window renders internal/core and does nothing else: it holds no wallpaper
logic of its own and reaches no further than the core, which is the rule that
keeps it in step with the CLI (project rule: CLI/GUI parity).

Every core call runs off the UI thread and hops back with fyne.Do. Nothing
transient reflows the interface: result banners and the progress indicator
float over the content as popups, and the log panes are fixed-height lists.
*/
package gui

import (
	"context"
	"fmt"
	"image/color"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/markdown"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// section is one entry in the left navigation.
type section struct {
	title string
	icon  fyne.Resource
	build func(*ui) fyne.CanvasObject
}

// ui holds the window's widgets and everything it has loaded from the core.
type ui struct {
	app     fyne.App
	win     fyne.Window
	version string
	// configPath is the --config override, carried into every core request so
	// that the window and `clockwork-orange --config …` read the same document.
	configPath string
	// deps is the outside world for every core call; nil is production. Tests
	// supply fakes.
	deps *core.Deps

	// appearance is the scheme, interface font, text size and scale,
	// persisted across runs in the preference store. The console font is not
	// in it: that lives in the document (consoleFamily).
	appearance fdtheme.Appearance

	content *container.Scroll
	nav     *widget.List
	frame   *fyne.Container // holds the status bar, so it can be redrawn
	current int             // the selected section, so an operation can rebuild it

	// busyCount is how many operations are running. A count rather than a flag:
	// loading a section can start more than one, and the indicator must not go
	// out when the first of them finishes.
	busyCount int
	busyWhat  string
	// busyPop is the centred progress popup, up while busyCount > 0 once the
	// operation has run longer than busyPopDelay; busyLabel is its text.
	busyPop   *widget.PopUp
	busyLabel *widget.Label
	busySeq   int
	// busyCancel, when set, is offered as a Cancel button on the busy popup
	// (the history import is the one operation long enough to want it).
	busyCancel context.CancelFunc
	// flashPop carries the result banner over the content.
	flashPop *widget.PopUp

	// Loaded from the core on a goroutine, read and written on the UI thread.
	//
	// Each has a companion flag rather than being tested for emptiness:
	// "loaded and empty" and "not loaded" are different states.
	doc       config.Document
	docPath   string
	docExists bool
	docOK     bool
	service   core.ServiceStatusResult
	serviceOK bool
	blacklist []store.BlacklistItem
	blOK      bool
	history   store.HistoryStats
	histOK    bool
	// plugins is the registry with schemas, fetched once: it needs no store
	// and does not change while the window is open.
	plugins []core.PluginInfo

	// dirty marks the document as edited but not yet written; the auto-save
	// timer (save.go) writes it 1 s after the last change (R7.9).
	saveTimer *time.Timer
	saveMu    sync.Mutex
	saveSeq   int
	saved     func() // test hook, called after each write

	// activity is the log every long-running thing in this window writes to:
	// the wallpaper timer's cycles, plugin runs started from a section, and on
	// Linux the service journal tail. It outlives the sections.
	activity *logpane.Pane
	// timer is the in-process wallpaper rotation (R7.12).
	timer wallpaperTimer
	// review is the plugin section's image review, kept across rebuilds of
	// the section on screen.
	review *reviewModel
	// readme is the About section's document pane while that section is on
	// screen. It watches the content scroll, so detach forgets it.
	readme *markdown.Pane
	// blState is the Blacklist section's filter and selection.
	blState blacklistState
	// pluginTab is the plugin sections' selected tab (0 configuration, 1
	// review), kept across rebuilds and shared by every plugin section.
	pluginTab int
	// journalStop ends the Service section's auto-refresh ticker.
	journalStop func()
	// runs guards one plugin run at a time from the run dialog.
	running bool
	// hiddenToTray says the window is hidden rather than closed.
	hiddenToTray bool
	// release frees the single-instance lock on quit; stopListen closes the
	// second-launch socket.
	release    func()
	stopListen func()
	// showHook replaces showWindow for the second-launch listener in tests.
	showHook func()
	// tick drives the 5 s status polls while the window is open.
	tickStop func()
	// lastSize is the window size last persisted, so the poll writes only on
	// a real change.
	lastSize fyne.Size

	// flashes is the result-banner slot: one banner at a time, in a region of
	// the window that keeps its height whether or not anything is in it.
	flashes  *fyne.Container
	flashSeq int // identifies the banner that owns the slot, so a stale timer cannot clear a newer one
}

// loadAppearance reads the saved appearance, falling back to the defaults.
func (u *ui) loadAppearance() {
	u.appearance = fdtheme.LoadAppearance(u.app.Preferences())
}

// applyAppearance saves the current appearance and rebuilds the theme from it.
func (u *ui) applyAppearance() {
	u.appearance.Save(u.app.Preferences())
	u.app.Settings().SetTheme(u.theme())
}

// theme is the current theme: scheme, interface font and text size from the
// preference store, console font from the document. Built here rather than by
// Appearance.Theme because the monospace face comes from the YAML, not from
// the preference store.
func (u *ui) theme() fdtheme.Theme {
	a := u.appearance
	return fdtheme.New(fdtheme.SchemeByName(a.Scheme), fdtheme.Options{
		Font:     fdtheme.LoadFont(a.Font),
		Mono:     fdtheme.LoadFont(consoleFamily(u.doc)),
		TextSize: a.TextSize,
	})
}

// consoleFamily is the document's console font family; the Python default
// "Monospace" is not an installed family, so it (and empty) mean Fyne's face.
func consoleFamily(doc config.Document) string {
	if doc.ConsoleFontFamily == "" || doc.ConsoleFontFamily == consoleFontDefault {
		return ""
	}
	return doc.ConsoleFontFamily
}

// consoleSize is the log panes' text size (console_font_size, default 10),
// clamped to the range the Python spin box allowed.
func consoleSize(doc config.Document) float32 {
	n := docDefaultsFor(doc).ConsoleFontSize
	return float32(min(48, max(6, n)))
}

// paneOptions shapes a log pane the way every pane in this window is drawn:
// the fixed height, the console text size from the document, Copy through the
// app's clipboard reporting into the banner slot, and Clear when onClear is
// given (nil for a pane whose content comes from the journal).
func (u *ui) paneOptions(title string, onClear func()) logpane.Options {
	return logpane.Options{
		Title:     title,
		Height:    logpane.DefaultHeight,
		TextSize:  consoleSize(u.doc),
		OnClear:   onClear,
		Clipboard: u.app.Clipboard(),
		Flash:     u.flash,
	}
}

// The fixed section titles (R7.3). The first is Service on Linux and Activity
// elsewhere; one section per registered plugin sits between it and History.
const (
	sectionService    = "Service"
	sectionActivity   = "Activity"
	sectionHistory    = "History"
	sectionBlacklist  = "Blacklist"
	sectionSettings   = "Settings"
	sectionAppearance = "Appearance"
	sectionAbout      = "About"
)

// firstSectionTitle is Service where systemd is, Activity where it is not.
func firstSectionTitle() string {
	if runtime.GOOS == "linux" {
		return sectionService
	}
	return sectionActivity
}

// pluginTitle is the plugin's navigation entry: `_` → space, Title Case, as
// main_window.py did.
func pluginTitle(name string) string {
	words := strings.Fields(strings.ReplaceAll(name, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// pluginForTitle is the inverse, for a section title that names a plugin.
func pluginForTitle(title string) (string, bool) {
	for _, name := range core.AvailablePluginNames() {
		if pluginTitle(name) == title {
			return name, true
		}
	}
	return "", false
}

// sectionTitles is the navigation in order.
//
// The order and the names live here, apart from the icons, because a theme icon
// cannot be constructed before an app exists: asking for one first makes Fyne
// log "Attempt to access current Fyne app when none is started", which is what
// the flag help would print, since it lists these. Plugin titles come from the
// registry, which needs no store.
func sectionTitles() []string {
	out := []string{firstSectionTitle()}
	for _, name := range core.AvailablePluginNames() {
		out = append(out, pluginTitle(name))
	}
	return append(out, sectionHistory, sectionBlacklist, sectionSettings, sectionAppearance, sectionAbout)
}

// sectionBuilders is what each section is made of. sections() walks
// sectionTitles and looks each one up here, so a title with no builder is a
// missing section rather than a silently different list from the one --section
// is told about.
//
// A function rather than a package variable: the builders reach back to
// sections() when a section rebuilds itself, and Go reports that as an
// initialization cycle in a package-level map.
func sectionBuilders() map[string]struct {
	icon  func() fyne.Resource
	build func(*ui) fyne.CanvasObject
} {
	type entry = struct {
		icon  func() fyne.Resource
		build func(*ui) fyne.CanvasObject
	}
	m := map[string]entry{
		sectionService:    {theme.ComputerIcon, (*ui).buildService},
		sectionActivity:   {theme.ComputerIcon, (*ui).buildActivity},
		sectionHistory:    {theme.HistoryIcon, (*ui).buildHistory},
		sectionBlacklist:  {theme.CancelIcon, (*ui).buildBlacklist},
		sectionSettings:   {theme.SettingsIcon, (*ui).buildSettings},
		sectionAppearance: {theme.ColorPaletteIcon, (*ui).buildAppearance},
		sectionAbout:      {theme.HelpIcon, (*ui).buildAbout},
	}
	for _, name := range core.AvailablePluginNames() {
		m[pluginTitle(name)] = entry{theme.FileImageIcon, func(u *ui) fyne.CanvasObject { return u.buildPlugin(name) }}
	}
	return m
}

func sections() []section {
	builders := sectionBuilders()
	out := make([]section, 0, 12)
	for _, title := range sectionTitles() {
		b, ok := builders[title]
		if !ok {
			continue // a title with no builder draws nothing; see SectionNames
		}
		out = append(out, section{title: title, icon: b.icon(), build: b.build})
	}
	return out
}

// SectionNames lists the navigation entries, for --section and for a capture
// script to iterate.
//
// Reads the titles rather than building the sections: this is called while
// parsing flags, before there is an app to hang an icon on.
func SectionNames() []string { return sectionTitles() }

// SchemeNames lists the colour schemes, for the same reason.
func SchemeNames() []string { return fdtheme.SchemeNames() }

// Options configure a run. Section and Scheme exist so a capture script can
// deep-link into the window; they override the saved appearance for that run
// without saving over it.
type Options struct {
	// ConfigPath is the --config override; empty uses the default document.
	ConfigPath string
	Section    string // navigation entry to open on; empty means the first
	Scheme     string // color scheme to force; empty means the saved one
}

// Window geometry (R7.2): the default when the config has no size, and how
// often the size is checked for persisting.
const (
	defaultWindowWidth  = 1180
	defaultWindowHeight = 760
	sizePollInterval    = 500 * time.Millisecond
	statusPollInterval  = 5 * time.Second
)

/*
Run opens the window and blocks until it is closed.

A second instance asks the first to show its window and exits (R7.11, R7.19):
a second window would start a second wallpaper timer, and a launch that does
nothing because the first window is hidden in the tray is a dead end.
*/
func Run(o Options) {
	release, ok := core.TryGUILock()
	if !ok {
		requestShow()
		return
	}
	// Before the toolkit starts: GLFW reads the cursor theme from the
	// environment at init, and there is no second chance once the window is up.
	fdtheme.ApplyCursorTheme()

	// The ID gives the app a preferences store, which Fyne writes under the
	// user's config directory. That file holds the appearance settings and
	// nothing else: every setting the CLI can also see lives in
	// clockwork-orange.yml, so that the daemon and this window agree.
	a := app.NewWithID("io.ushineko.clockwork-orange")
	u := &ui{app: a, version: core.Version(), configPath: o.ConfigPath, release: release}
	u.activity = logpane.New(nil)
	u.win = a.NewWindow("Clockwork Orange " + u.version)
	u.win.SetIcon(appIcon())
	a.SetIcon(appIcon())
	u.loadAppearance()
	fdtheme.ApplyScale(u.appearance.Scale)
	if o.Scheme != "" {
		// Forced for this run only, so a capture does not overwrite whatever
		// the user had chosen.
		u.appearance.Scheme = fdtheme.SchemeByName(o.Scheme).Name
	}
	// The document is read before the theme is applied and the window sized:
	// the console font, the window's size and every form come out of it.
	u.loadConfigNow()
	u.plugins = core.AvailablePlugins()
	if o.Scheme != "" {
		u.app.Settings().SetTheme(u.theme())
	} else {
		u.applyAppearance()
	}

	u.content = container.NewScroll(widget.NewLabel(""))
	u.flashes = container.NewVBox()
	secs := sections()

	u.nav = widget.NewList(
		func() int { return len(secs) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewIcon(theme.HomeIcon()), widget.NewLabel("placeholder"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*fyne.Container)
			row.Objects[0].(*widget.Icon).SetResource(secs[i].icon)
			row.Objects[1].(*widget.Label).SetText(secs[i].title)
		},
	)
	u.nav.OnSelected = func(i widget.ListItemID) {
		u.current = i
		u.swap(secs[i].build, false)
	}

	split := container.NewHSplit(u.nav, u.content)
	split.SetOffset(0.16)

	// Result banners and the progress indicator float over the content as
	// popups (see flash and busy), so nothing below the header reflows when
	// an operation starts, finishes or reports. The frame is the status bar.
	u.frame = container.NewVBox(u.statusBar())
	u.win.SetContent(container.NewBorder(u.header(), u.frame, nil, nil, split))
	// F5 and Ctrl+R reload. The config is edited by the CLI's --write-config
	// and by hand, and the download directories fill up behind our back.
	u.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl},
		func(fyne.Shortcut) { u.invalidate() })
	u.win.Canvas().SetOnTypedKey(u.onTypedKey)

	u.win.Resize(windowSize(u.doc, u.docExists))
	u.nav.Select(sectionIndex(secs, o.Section))
	u.setupTray()
	u.stopListen = u.listenShow()
	u.win.SetCloseIntercept(u.onClose)
	u.loadService()
	u.startPolling()
	u.timer.start(u)
	u.notify("Clockwork Orange", "Application started")
	u.win.SetMaster()
	u.win.ShowAndRun()
	u.shutdown()
}

// windowSize is the size to open at: the config's window_width/height when
// the file exists and has them, else the default (R7.2). The Python restored
// 800×600 from a missing file; that default is the GUI's own and the larger
// one fits the sections this window has.
func windowSize(doc config.Document, exists bool) fyne.Size {
	if exists && doc.WindowWidth > 0 && doc.WindowHeight > 0 {
		return fyne.NewSize(float32(doc.WindowWidth), float32(doc.WindowHeight))
	}
	return fyne.NewSize(defaultWindowWidth, defaultWindowHeight)
}

// sectionIndex resolves a section name to its position. An unknown name opens
// the first section rather than failing: a typo in a capture script should
// produce a wrong screenshot, which is obvious, not a dead window.
func sectionIndex(secs []section, name string) int {
	if name == "" {
		return 0
	}
	for i, s := range secs {
		if strings.EqualFold(s.title, name) {
			return i
		}
	}
	return 0
}

// selectSection moves the navigation to a named section.
func (u *ui) selectSection(name string) {
	if u.nav == nil {
		return
	}
	u.nav.Select(sectionIndex(sections(), name))
}

// currentTitle is the section on screen.
func (u *ui) currentTitle() string {
	secs := sections()
	if u.current >= 0 && u.current < len(secs) {
		return secs[u.current].title
	}
	return ""
}

// rebuild redraws the whole window, including the status bar. refresh alone
// only replaces the content pane.
func (u *ui) rebuild() {
	if u.frame != nil {
		u.frame.Objects[frameStatusBar] = u.statusBar()
		u.frame.Refresh()
	}
	u.refresh()
}

// refresh rebuilds the current section, so a view picks up what an operation
// just changed. Called on the UI thread.
//
// A nil content pane means there is no window to draw into: a headless test, or
// a load that finished after the window closed. Both are ordinary, and building
// a section for nobody would start the loads that section asks for.
func (u *ui) refresh() {
	if u.content == nil {
		return
	}
	secs := sections()
	if u.current >= 0 && u.current < len(secs) {
		u.swap(secs[u.current].build, true)
	}
}

/*
swap replaces the content pane with a freshly built section.

The live widgets are dropped before the new section is built, not after: built
first and dropped afterwards, a section registers its brand-new list and then
has it thrown away by the very call that put it on screen.

keepScroll is for a rebuild of the section already on screen: every operation
rebuilds it when it starts and when it stops (regate), and a rebuild that
scrolled to the top threw the reader away from the slider they had just moved.
Navigating to a section starts at its top.
*/
func (u *ui) swap(build func(*ui) fyne.CanvasObject, keepScroll bool) {
	if u.content == nil {
		return
	}
	u.detach()
	u.show(build(u), keepScroll)
}

// detach forgets every live widget the sections hold, and stops the review
// watcher: the pane they drew into is about to be replaced.
func (u *ui) detach() {
	u.activity.Detach()
	if u.review != nil {
		u.review.detach()
	}
	if u.readme != nil {
		u.readme.Detach()
		u.readme = nil
	}
}

func (u *ui) show(o fyne.CanvasObject, keepScroll bool) {
	if u.content == nil {
		return
	}
	offset := u.content.Offset
	u.content.Content = o
	u.content.Refresh()
	if !keepScroll {
		u.content.ScrollToTop()
		return
	}
	u.content.Offset = offset
	u.content.Refresh()
}

// header is the window's title strip. It carries Refresh because the config
// and the download directories are changed by the daemon and by the CLI
// without telling us.
func (u *ui) header() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Clockwork Orange", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	reload := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { u.invalidate() })
	bar := container.NewHBox(title, layout.NewSpacer(), reload)
	return container.NewVBox(container.NewPadded(bar), widget.NewSeparator())
}

// statusBar names the mode, the interval, the enabled plugins and, on Linux,
// the service state, from every section.
func (u *ui) statusBar() fyne.CanvasObject {
	mode := widget.NewLabel(modeText(core.ResolveMode(u.doc, false, false)))
	wait := widget.NewLabel(fmt.Sprintf("%ds", waitSeconds(u.doc)))
	enabled := u.doc.EnabledPlugins()
	plugins := widget.NewLabel(fmt.Sprintf("%d/%d", len(enabled), len(core.AvailablePluginNames())))

	bar := container.NewHBox(
		widgets.Dim("mode"), mode, widgets.Sep(),
		widgets.Dim("interval"), wait, widgets.Sep(),
		widgets.Dim("plugins"), plugins,
	)
	if runtime.GOOS == "linux" {
		svc := widgets.StatusText("reading…", fd.StatusInfo)
		if u.serviceOK {
			svc = widgets.StatusText(string(u.service.State), serviceStatus(u.service.State))
		}
		bar.Add(widgets.Sep())
		bar.Add(widgets.Dim("service"))
		bar.Add(svc)
	}
	if u.timer.daemonHeld {
		bar.Add(widgets.Sep())
		bar.Add(widgets.StatusText("timer idle: the daemon is cycling", fd.StatusInfo))
	}
	return container.NewVBox(widget.NewSeparator(), container.NewPadded(bar))
}

// modeText names the mode the way Settings labels it.
func modeText(m core.Mode) string {
	switch m {
	case core.ModeDual:
		return "desktop + lock screen"
	case core.ModeLockscreen:
		return "lock screen"
	case core.ModeDesktop:
		return "desktop"
	default:
		return "desktop (default)"
	}
}

// waitSeconds is the timer interval: default_wait, or the GUI default.
func waitSeconds(doc config.Document) int {
	if doc.DefaultWait > 0 {
		return doc.DefaultWait
	}
	return config.DefaultWaitGUI
}

/*
busy shows an indeterminate progress indicator until the returned function is
called.

Safe to call from a goroutine: it hops to the UI thread itself, and so does the
function it returns. EVERY core call gets one: a window that sits still with no
explanation reads as frozen, and the button that looks like it did nothing is
the button that gets clicked twice.
*/
func (u *ui) busy(what string) func() {
	fyne.Do(func() {
		u.busyCount++
		u.busyWhat = what
		u.showBusy()
		if u.busyCount == 1 {
			u.regate()
		}
	})

	var once sync.Once
	return func() {
		once.Do(func() {
			fyne.Do(func() {
				u.busyCount--
				if u.busyCount <= 0 {
					u.busyCount, u.busyWhat = 0, ""
					u.busyCancel = nil
					u.hideBusy()
					u.regate()
				}
			})
		})
	}
}

// busyPopDelay is how long an operation runs before the progress popup
// appears. Most operations finish inside it, and a popup that blinks for a
// tenth of a second on every click is worse than none.
const busyPopDelay = 300 * time.Millisecond

/*
showBusy puts the progress popup up, centred and modal, once the operation has
lasted long enough to deserve one. It names the operation, in the words the
caller of busy gave it, and offers Cancel when the operation can be cancelled.
*/
func (u *ui) showBusy() {
	if !u.onScreen() {
		return
	}
	if u.busyPop != nil {
		u.busyLabel.SetText(u.busyWhat)
		return
	}
	u.busySeq++
	seq := u.busySeq
	go func() {
		time.Sleep(busyPopDelay)
		fyne.Do(func() {
			if u.busySeq != seq || u.busyCount == 0 || u.busyPop != nil {
				return
			}
			u.busyLabel = widget.NewLabel(u.busyWhat)
			u.busyLabel.Alignment = fyne.TextAlignCenter
			bar := widget.NewProgressBarInfinite()
			rows := container.NewVBox(u.busyLabel, widgets.FixedWidth(bar, 320))
			if u.busyCancel != nil {
				cancel := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
					if u.busyCancel != nil {
						u.busyCancel()
					}
				})
				rows.Add(container.NewCenter(cancel))
			}
			u.busyPop = widget.NewModalPopUp(container.NewPadded(rows), u.win.Canvas())
			u.busyPop.Show()
		})
	}()
}

// hideBusy takes the progress popup down.
func (u *ui) hideBusy() {
	u.busySeq++ // a pending showBusy timer finds a different sequence and stops
	if u.busyPop != nil {
		u.busyPop.Hide()
		u.busyPop, u.busyLabel = nil, nil
	}
}

/*
regate rebuilds the current section when work starts and when it stops.

Every section disables the buttons that start work while something is running,
and it does that as it is built -- so a section built while an operation was in
flight comes out with dead buttons and nothing turns them back on. Rebuilding
from state is the fix rather than walking a list of buttons, because "disabled"
has several causes at once and only the builder knows all of them.
*/
func (u *ui) regate() {
	u.refresh()
	u.redrawStatus()
}

// working reports whether a core operation is in flight: one at a time, with
// the other buttons disabled rather than hidden, so the window does not change
// shape as work starts and finishes.
func (u *ui) working() bool { return u.busyCount > 0 || u.running }

// gate disables buttons while an operation runs.
func (u *ui) gate(buttons ...*widget.Button) {
	for _, b := range buttons {
		if u.working() {
			b.Disable()
		}
	}
}

// redrawStatus repaints the status bar and nothing else.
func (u *ui) redrawStatus() {
	if u.frame == nil {
		return
	}
	u.frame.Objects[frameStatusBar] = u.statusBar()
	u.frame.Refresh()
}

// The banner's geometry and timings.
const (
	// frameStatusBar is the status bar's position in u.frame.
	frameStatusBar = 0
	// flashWidth is how wide a banner is drawn, so a long message wraps
	// rather than spanning the window.
	flashWidth = 720
	// How long a banner stays before it starts fading. A warning gets longer
	// because it usually names a condition to act on.
	flashHoldGood = 6 * time.Second
	flashHoldWarn = 12 * time.Second
	// The fade itself. Long enough to read as intentional, short enough that
	// the banner is not sitting there half-gone.
	flashFade = 700 * time.Millisecond
)

// flashHold says how long a banner of this status stays up, and whether it goes
// on its own at all.
//
// A failure does not: it waits to be dismissed, or until another operation
// replaces it. An error that removes itself on a timer is an error nobody read,
// and the operation it describes has already not happened.
func flashHold(st fd.Status) (time.Duration, bool) {
	switch st {
	case fd.StatusBad:
		return 0, false
	case fd.StatusWarn:
		return flashHoldWarn, true
	default:
		return flashHoldGood, true
	}
}

/*
flash reports the result of an operation as a banner floated over the bottom of
the content, centred. One banner shows at a time: a newer result replaces an
older one rather than stacking, so the most recent thing that happened is always
the thing on screen, and nothing in the section behind it moves.

Fyne animates properties, not opacity: a widget has no alpha to fade. So the
fade is on the banner's own background rectangle, whose colour animates from the
status tint to fully transparent. The text is left at full strength for the whole
life of the banner, which is the accessible choice anyway.

Plugin output does not come through here. A run emits dozens of lines and one
banner per line would be a slot flickering for a minute; the log pane is where
those go, and one banner summarises the result.
*/
func (u *ui) flash(text string, st fd.Status) {
	u.flashSeq++
	seq := u.flashSeq

	tint := u.flashTint(st)
	bg := canvas.NewRectangle(tint)
	bg.CornerRadius = 2

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord

	// Dismissable, because a banner that only leaves on a timer leaves either
	// too early to read or too late to be rid of. This is also the only way to
	// clear a failure, which does not go on its own.
	dismiss := widget.NewButtonWithIcon("", theme.CancelIcon(), func() { u.clearFlash(seq) })
	dismiss.Importance = widget.LowImportance

	banner := container.NewStack(bg, container.NewPadded(
		container.NewBorder(nil, nil, widgets.Marker(st), dismiss, label)))
	u.flashes.Objects = []fyne.CanvasObject{banner}
	u.flashes.Refresh()
	u.showFlashPop()

	hold, fades := flashHold(st)
	if !fades || !u.onScreen() {
		// Nothing to fade with no window: the timer would come back seconds
		// later to animate a rectangle nobody is drawing, on a goroutine the
		// test that made it has long since finished with.
		return
	}

	transparent := color.NRGBA{R: tint.R, G: tint.G, B: tint.B, A: 0}
	go func() {
		time.Sleep(hold)
		fyne.Do(func() {
			if u.flashSeq != seq {
				return // a newer banner owns the slot
			}
			fade := canvas.NewColorRGBAAnimation(tint, transparent, flashFade, func(c color.Color) {
				bg.FillColor = c
				canvas.Refresh(bg)
			})
			fade.Curve = fyne.AnimationEaseIn
			fade.Start()
		})
		time.Sleep(flashFade)
		fyne.Do(func() { u.clearFlash(seq) })
	}()
}

// clearFlash empties the slot, unless a newer banner has taken it. Called from
// the dismiss button and from the fade's own timer, which may arrive after the
// banner it belongs to has already been replaced.
func (u *ui) clearFlash(seq int) {
	if u.flashSeq != seq {
		return
	}
	u.flashes.Objects = nil
	u.flashes.Refresh()
	if u.flashPop != nil {
		u.flashPop.Hide()
	}
}

// showFlashPop floats the banner over the content, centred, a little above the
// status bar. Not modal: a result is something to read, not something to
// answer, and the section behind it stays usable.
func (u *ui) showFlashPop() {
	if !u.onScreen() {
		return
	}
	c := u.win.Canvas()
	if u.flashPop == nil {
		u.flashPop = widget.NewPopUp(widgets.FixedWidth(u.flashes, flashWidth), c)
	}
	cs := c.Size()
	width := min(float32(flashWidth), cs.Width-40)
	u.flashPop.Content = widgets.FixedWidth(u.flashes, width)
	size := u.flashPop.Content.MinSize()
	pos := fyne.NewPos((cs.Width-size.Width)/2, cs.Height-size.Height-56)
	u.flashPop.ShowAtPosition(pos)
}

// flashTint is the banner's starting colour: the status role from the active
// scheme, at low alpha so text stays readable over it in every scheme.
func (u *ui) flashTint(st fd.Status) color.NRGBA {
	p := fdtheme.SchemeByName(u.appearance.Scheme)
	var c color.Color
	switch st {
	case fd.StatusGood:
		c = p.Positive
	case fd.StatusWarn:
		c = p.Neutral
	case fd.StatusBad:
		c = p.Negative
	default:
		c = p.SelectionBG
	}
	tint, _ := fdtheme.Alpha(c, 0x4d).(color.NRGBA)
	return tint
}

// notify sends a desktop notification (R7.11) that goes away on its own.
// Under the test driver nothing is sent: the test app has no notification
// centre and the D-Bus call would reach the developer's desktop.
func (u *ui) notify(title, body string) {
	if u.app == nil || !u.onScreen() {
		return
	}
	sendNotification(u, title, body)
}

func fyneNotification(title, body string) *fyne.Notification {
	return fyne.NewNotification(title, body)
}

/*
Actions is every operation this window can reach, named by the CLI command it
corresponds to.

It exists for the parity test in tests/parity: that test walks the cobra command
tree and this list and fails when either holds an operation the other does not.
Adding a command without a GUI affordance breaks the build, which is the point —
the two front ends drift silently otherwise.

A name here is a claim that the operation is reachable and wired, not that a
button exists. Do not add one to quiet the test.
*/
func Actions() []string {
	return []string{
		// Service (Linux): the status line, the toolbar and the journal pane
		"service status", "service start", "service stop", "service restart",
		"service install", "service uninstall", "service logs",
		// Plugin sections: the enable check and every schema field (plugins
		// list), Download Now / Reset & Run (plugin run), and review mode's
		// Apply Blacklist (blacklist add)
		"plugins list", "plugin run", "blacklist add",
		// History
		"history stats", "history clear", "history import",
		// Blacklist
		"blacklist list", "blacklist remove",
		// Settings: the Raw YAML view
		"config show",
	}
}
