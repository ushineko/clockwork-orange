/*
Package gui is the desktop front end of spec 010 (R7).

The window renders internal/core and does nothing else: it holds no wallpaper
logic of its own and reaches no further than the core, which is the rule that
keeps it in step with the CLI (project rule: CLI/GUI parity).

The window itself (navigation, content pane, status bar, busy indicator and
result banners) is fynedesygn's shell; this package supplies the sections, the
status bar's segments, the wallpaper timer, the tray and the auto-save. Every
core call runs off the UI thread and hops back with fyne.Do. Nothing transient
reflows the interface: result banners and the progress indicator float over
the content as popups, and the log panes are fixed-height lists.
*/
package gui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/logpane"
	"github.com/ushineko/fynedesygn/markdown"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/clockwork-orange/internal/config"
	"github.com/ushineko/clockwork-orange/internal/core"
	"github.com/ushineko/clockwork-orange/internal/store"
)

// appID names the preference store and, on Wayland, the window's app_id,
// which the compositor matches to the desktop entry of the same basename.
const appID = "io.ushineko.clockwork-orange"

// ui holds the program's state and the shell that draws it.
type ui struct {
	// sh is the window: navigation, content pane, status bar, busy indicator
	// and banners. The shell builds the first section and calls OnStart
	// before New returns it, so the builders and hooks take it from their
	// argument and record it here rather than reading a field set later.
	sh      *shell.Shell
	version string
	// configPath is the --config override, carried into every core request so
	// that the window and `clockwork-orange --config …` read the same document.
	configPath string
	// deps is the outside world for every core call; nil is production. Tests
	// supply fakes.
	deps *core.Deps

	// Loaded from the core on a goroutine, read and written on the UI thread.
	//
	// Each has a companion flag rather than being tested for emptiness:
	// "loaded and empty" and "not loaded" are different states.
	doc       config.Document
	docPath   string
	docExists bool
	docOK     bool
	// docErr is the banner a failed read of the document deserves, kept until
	// there is a window to show it in.
	docErr    string
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
	// timer (state.go) writes it 1 s after the last change (R7.9).
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
	// running guards one plugin run at a time from the run dialog; the shell
	// counts it as work through Options.AlsoWorking.
	running bool
	// hiddenToTray says the window is hidden rather than closed.
	hiddenToTray bool
	// release frees the single-instance lock on quit; stopListen closes the
	// second-launch socket; stopPprof closes the profiling server when one was
	// asked for.
	release    func()
	stopListen func()
	stopPprof  func()
	// showHook replaces showWindow for the second-launch listener in tests.
	showHook func()
	// tick drives the 5 s status polls while the window is open.
	tickStop func()
	// lastSize is the window size last persisted, so the poll writes only on
	// a real change.
	lastSize fyne.Size
}

// themeFor is the shell's Theme hook: scheme, interface font and text size
// from the appearance (the preference store), console font from the document.
// The shell calls it at start and on every SetAppearance; a console font
// change re-applies the current appearance to reach it.
func (u *ui) themeFor(a fdtheme.Appearance) fyne.Theme {
	return fdtheme.New(fdtheme.SchemeByName(a.Scheme), fdtheme.Options{
		Font:     fdtheme.LoadFont(a.Font),
		Mono:     fdtheme.LoadFont(consoleFamily(u.doc)),
		TextSize: a.TextSize,
	})
}

// setAppearance saves and applies the appearance through the shell, which
// builds the theme with themeFor.
func (u *ui) setAppearance(a fdtheme.Appearance) { u.sh.SetAppearance(a) }

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
		Clipboard: u.sh.App.Clipboard(),
		Flash:     u.sh.Flash,
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

// sectionEntry is what a section is made of: a deferred icon, its builder, the
// hook that releases the live widgets it holds when it is replaced, and the
// hook that runs when the navigation arrives at it.
type sectionEntry struct {
	icon   func() fyne.Resource
	build  func(*ui) fyne.CanvasObject
	detach func(*ui)
	// arrive runs when the navigation arrives at the section, and not when it
	// is rebuilt where it stands (spec 013 R7). A section that refetches in
	// its builder loops, because the fetch finishing rebuilds the section that
	// started it; a section that refetches only when its loaded flag happens
	// to be false shows whatever it last read, which is stale the moment a
	// plugin run blacklists an image.
	arrive func(*ui)
}

// sectionBuilders is what each section is made of. sections walks
// sectionTitles and looks each one up here, so a title with no builder is a
// missing section rather than a silently different list from the one --section
// is told about.
//
// A function rather than a package variable: the builders reach back to the
// sections when a section rebuilds itself, and Go reports that as an
// initialization cycle in a package-level map.
func sectionBuilders() map[string]sectionEntry {
	// The activity pane is drawn by Service and Activity alike; the review
	// belongs to whichever plugin section is on screen; the README pane to
	// About. Each is told before its section is replaced.
	activity := func(u *ui) { u.activity.Detach() }
	review := func(u *ui) {
		if u.review != nil {
			u.review.detach()
		}
	}
	readme := func(u *ui) {
		if u.readme != nil {
			u.readme.Detach()
			u.readme = nil
		}
	}
	// Arriving at one of the two stores is a reason to read it again: both
	// are written behind the section's back, by a plugin run marking an image
	// or by the CLI.
	staleBlacklist := func(u *ui) { u.blOK = false }
	staleHistory := func(u *ui) { u.histOK = false }
	m := map[string]sectionEntry{
		sectionService:    {theme.ComputerIcon, (*ui).buildService, activity, nil},
		sectionActivity:   {theme.ComputerIcon, (*ui).buildActivity, activity, nil},
		sectionHistory:    {theme.HistoryIcon, (*ui).buildHistory, nil, staleHistory},
		sectionBlacklist:  {theme.CancelIcon, (*ui).buildBlacklist, nil, staleBlacklist},
		sectionSettings:   {theme.SettingsIcon, (*ui).buildSettings, nil, nil},
		sectionAppearance: {theme.ColorPaletteIcon, (*ui).buildAppearance, nil, nil},
		sectionAbout:      {theme.HelpIcon, (*ui).buildAbout, readme, nil},
	}
	for _, name := range core.AvailablePluginNames() {
		m[pluginTitle(name)] = sectionEntry{pluginIcon(name), func(u *ui) fyne.CanvasObject { return u.buildPlugin(name) }, review, nil}
	}
	return m
}

/*
AffixedActions names, per section, the controls that must not scroll out of
view (spec 014).

The rule: every control that starts, cancels or commits work occupies the same
place in its section however much of the section is scrolled. What scrolls is
the material the control acts on -- the form, the table, the statistics, the
prose -- never the control itself. A section holding one is a Border with the
controls in a fixed edge and a scroller in the centre, not a column that
happens to fit the window it was built on.

This is a list rather than a rule the code can infer, because "a control that
starts work" is not something a walker can tell from a button: the plugin
form's search terms carry a remove button per row, and those belong to their
row and scroll with it. Naming them is also the point -- tests/parity names
every operation for the same reason. A label here is a claim that the control
is affixed, and TestTheAffixedControlsDoNotScroll holds the window to it.

Matched by prefix, because three of these labels carry a count.
*/
func AffixedActions() map[string][]string {
	plugin := []string{"Download now", "Reset & run"}
	m := map[string][]string{
		sectionService:    {"Start", "Stop", "Restart", "Install", "Uninstall", "Refresh now"},
		sectionHistory:    {"Reset history database", "Scan & import existing files"},
		sectionBlacklist:  {"Remove selected from blacklist"},
		sectionAppearance: {"Reset to defaults"},
	}
	for _, name := range core.AvailablePluginNames() {
		m[pluginTitle(name)] = plugin
	}
	return m
}

// sections is the navigation as the shell takes it. The builders record the
// shell they are handed: the shell builds the first section before New has
// returned it to Run, so it cannot be read from u.sh at that moment. A nil u
// is enough for the titles, which is all SectionNames needs.
func sections(u *ui) []shell.Section {
	builders := sectionBuilders()
	out := make([]shell.Section, 0, 12)
	for _, title := range sectionTitles() {
		b, ok := builders[title]
		if !ok {
			continue // a title with no builder draws nothing; see SectionNames
		}
		sec := shell.NewSection(title, b.icon, func(*shell.Shell) fyne.CanvasObject { return b.build(u) })
		if b.detach != nil {
			sec.OnDetach(func() { b.detach(u) })
		}
		if b.arrive != nil {
			sec.OnArrive(func() { b.arrive(u) })
		}
		out = append(out, sec)
	}
	return out
}

// SectionNames lists the navigation entries, for --section and for a capture
// script to iterate.
//
// Reads the titles rather than building the sections: this is called while
// parsing flags, before there is an app to hang an icon on.
func SectionNames() []string { return shell.Names(sections(nil)) }

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
	// Before anything allocates: a soft ceiling, so a review of 4K images
	// collects harder instead of leaving the arena at its high-water mark
	// (spec 018).
	setMemLimit()
	u := &ui{version: core.Version(), configPath: o.ConfigPath, release: release, activity: logpane.New(nil)}
	// The document is read before the shell is built: the window's size, the
	// console font and every form come out of it.
	u.loadConfigNow()
	u.plugins = core.AvailablePlugins()
	shell.New(u.shellOptions(o))
	u.sh.Window.ShowAndRun()
	u.shutdown()
}

// settingsPath is this window's own settings file (spec 013 R2.1).
//
// In the state directory this program already owns, beside history.db and
// blacklist.db, rather than a second directory named for the app ID. It holds
// what belongs to the window and nothing else -- the appearance, the
// navigation's shape, the dragged dividers. Every setting the CLI can also see
// stays in clockwork-orange.yml, where 2.9.x and the daemon read it.
func settingsPath() string {
	return filepath.Join(config.StateDir(), "gui-settings.json")
}

// shellOptions describes this program to the shell.
func (u *ui) shellOptions(o Options) shell.Options {
	return shell.Options{
		AppID:        appID,
		Name:         "Clockwork Orange",
		Version:      u.version,
		Icon:         appIcon(),
		SettingsPath: settingsPath(),
		Sections:     sections(u),
		Section:      o.Section,
		Scheme:       o.Scheme,
		Size:         windowSize(u.doc, u.docExists),
		// Every shape the library offers (spec 013 R3.1): this window has
		// seven-odd sections and a review that wants the whole width, so
		// icons-only and hidden are both useful, and the shell draws the
		// control and binds Ctrl+B once a program lists more than one.
		NavModes:      []shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden},
		NavPlacements: []shell.NavPlacement{shell.NavLeft, shell.NavTop},
		StatusBar:     func(*shell.Shell) []fyne.CanvasObject { return u.statusSegments() },
		OnCreate:      func(s *shell.Shell) { u.sh = s },
		Theme:         u.themeFor,
		OnTypedKey:    u.onTypedKey,
		OnStart:       u.onStart,
		OnInvalidate:  u.onInvalidate,
		OnStop:        func(*shell.Shell) { u.shutdown() },
		AlsoWorking:   func() bool { return u.running },
	}
}

// onStart runs once the window exists and before it shows: the tray, the
// second-launch listener, the close intercept, the loads every section shows,
// the polls and the wallpaper timer.
func (u *ui) onStart(s *shell.Shell) {
	u.setupTray()
	u.stopListen = u.listenShow()
	// Off unless CLOCKWORK_PPROF asks for it (spec 017). Reported as a banner
	// rather than logged: a profiling server the user turned on and that did
	// not come up is worth saying out loud.
	u.stopPprof = startPprof(func(msg string) { s.Flash(msg, fd.StatusInfo) })
	s.Window.SetCloseIntercept(u.onClose)
	if u.docErr != "" {
		s.Flash(u.docErr, fd.StatusBad)
	}
	u.loadService()
	u.startPolling()
	u.timer.start(u)
	u.notify("Clockwork Orange", "Application started")
}

// onInvalidate discards everything loaded from the core, which makes the
// sections fetch again, and reloads what the status bar shows from every
// section. The shell rebuilds afterwards. Off screen, clearing the flags is
// the whole of the work: whatever builds the sections next will fetch.
//
// The activity log is deliberately not part of this: F5 while a plugin runs
// must not throw away the output it has produced so far.
func (u *ui) onInvalidate(s *shell.Shell) {
	u.docOK, u.serviceOK, u.blOK, u.histOK = false, false, false, false
	if !s.OnScreen() {
		return
	}
	u.loadConfigNow()
	u.loadService()
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

// statusSegments names the mode, the interval, the enabled plugins and, on
// Linux, the service state, from every section.
func (u *ui) statusSegments() []fyne.CanvasObject {
	mode := widget.NewLabel(modeText(core.ResolveMode(u.doc, false, false)))
	wait := widget.NewLabel(fmt.Sprintf("%ds", waitSeconds(u.doc)))
	enabled := u.doc.EnabledPlugins()
	plugins := widget.NewLabel(fmt.Sprintf("%d/%d", len(enabled), len(core.AvailablePluginNames())))

	segs := []fyne.CanvasObject{
		widgets.Dim("mode"), mode, widgets.Sep(),
		widgets.Dim("interval"), wait, widgets.Sep(),
		widgets.Dim("plugins"), plugins,
	}
	if runtime.GOOS == "linux" {
		svc := widgets.StatusText("reading…", fd.StatusInfo)
		if u.serviceOK {
			svc = widgets.StatusText(string(u.service.State), serviceStatus(u.service.State))
		}
		segs = append(segs, widgets.Sep(), widgets.Dim("service"), svc)
	}
	if u.timer.daemonHeld {
		segs = append(segs, widgets.Sep(), widgets.StatusText("timer idle: the daemon is cycling", fd.StatusInfo))
	}
	return segs
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

// notify sends a desktop notification (R7.11) that goes away on its own.
// Under the test driver nothing is sent: the test app has no notification
// centre and the D-Bus call would reach the developer's desktop.
func (u *ui) notify(title, body string) {
	if u.sh == nil || u.sh.App == nil || !u.sh.OnScreen() {
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
