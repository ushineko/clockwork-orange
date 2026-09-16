//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/ushineko/clockwork-orange/internal/events"
)

// WindowsPlatform sets the desktop through the spanned-composite technique
// (D10, R4.6-R4.8): one JPEG covering every monitor, applied with
// WallpaperStyle=22. The geometry and control flow live in stitch.go; this
// file is only the Win32 surface behind winDesktop.
type WindowsPlatform struct {
	Runner  Runner
	Events  events.Events
	desktop winDesktop
}

// New returns the Windows implementation for this host.
func New(r Runner) Platform { return &WindowsPlatform{Runner: r, desktop: win32Desktop{}} }

// NewWithEvents is New with a log sink.
func NewWithEvents(r Runner, ev events.Events) Platform {
	return &WindowsPlatform{Runner: r, Events: ev, desktop: win32Desktop{}}
}

// Name implements Platform.
func (*WindowsPlatform) Name() string { return "windows" }

// LockscreenSupported implements Platform.
func (*WindowsPlatform) LockscreenSupported() bool { return false }

// MonitorCount implements Platform (R4.6).
func (p *WindowsPlatform) MonitorCount(context.Context) int {
	monitors, err := p.desktop.monitors()
	if err != nil || len(monitors) == 0 {
		p.Events.Debugf("Failed to get monitor count, assuming 1: %v", err)
		return 1
	}
	return len(monitors)
}

// SetWallpaper implements Platform (R4.7): the multi-monitor path with one
// image, falling back to a plain SystemParametersInfoW when it fails.
func (p *WindowsPlatform) SetWallpaper(ctx context.Context, path string) error {
	abs := resolvePath(path)
	p.Events.Debugf("Setting Windows wallpaper: %s", abs)
	if !fileExists(abs) {
		p.Events.Errorf("File does not exist: %s", abs)
		return errNotExist(abs)
	}
	err := p.SetWallpaperMulti(ctx, []string{abs})
	if err == nil {
		return nil
	}
	p.Events.Debugf("Multi-monitor API failed, falling back to SystemParametersInfoW: %v", err)
	if err := p.desktop.applyWallpaper(abs); err != nil {
		p.Events.Errorf("Failed to set Windows wallpaper: %v", err)
		return fmt.Errorf("set Windows wallpaper: %w", err)
	}
	return nil
}

// SetWallpaperMulti implements Platform (R4.7).
func (p *WindowsPlatform) SetWallpaperMulti(_ context.Context, paths []string) error {
	if len(paths) == 0 {
		p.Events.Errorf("No image paths provided")
		return errors.New("no image paths provided")
	}
	resolved := make([]string, 0, len(paths))
	for _, raw := range paths {
		abs := resolvePath(raw)
		if !fileExists(abs) {
			p.Events.Errorf("File does not exist: %s", abs)
			continue
		}
		resolved = append(resolved, abs)
	}
	if len(resolved) == 0 {
		p.Events.Errorf("No valid image paths found")
		return errors.New("no valid image paths found")
	}
	if err := stitchAndApply(p.desktop, spannedDir(os.Getenv), resolved, p.Events); err != nil {
		p.Events.Errorf("Spanned wallpaper stitching failed: %v", err)
		return err
	}
	return nil
}

// SetLockscreen implements Platform: Windows has no supported lock-screen
// wallpaper API for an unprivileged process (R4.4).
func (p *WindowsPlatform) SetLockscreen(context.Context, string) error {
	p.Events.Infof("Lock screen wallpaper not supported on Windows.")
	return ErrUnsupported
}

// --- Win32 surface ---

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procEnumDisplayMonitors   = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW       = user32.NewProc("GetMonitorInfoW")
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	procSetAppUserModelID     = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

const (
	spiSetDeskWallpaper   = 0x0014
	spifUpdateIniFile     = 0x01
	spifSendWinIniChange  = 0x02
	desktopRegistryKey    = `Control Panel\Desktop`
	wallpaperStyleSpanned = "22"
)

type win32Rect struct{ Left, Top, Right, Bottom int32 }

type win32MonitorInfo struct {
	CbSize    uint32
	RcMonitor win32Rect
	RcWork    win32Rect
	DwFlags   uint32
}

// EnumDisplayMonitors takes a C callback; syscall.NewCallback may only be
// called a bounded number of times per process, so there is exactly one,
// and the results it appends to are guarded for the duration of a call.
var (
	enumMu      sync.Mutex
	enumResult  []Monitor
	enumMonitor = syscall.NewCallback(enumMonitorProc)
)

func enumMonitorProc(hMonitor, _ /* hdc */, _ /* lprcMonitor */, _ /* dwData */ uintptr) uintptr {
	var mi win32MonitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	r, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if r != 0 {
		rc := mi.RcMonitor
		enumResult = append(enumResult, Monitor{
			X: int(rc.Left), Y: int(rc.Top), W: int(rc.Right - rc.Left), H: int(rc.Bottom - rc.Top),
		})
	}
	return 1 // continue enumeration
}

// win32Desktop is the production winDesktop.
type win32Desktop struct{}

func (win32Desktop) monitors() ([]Monitor, error) {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumResult = nil
	r, _, e := procEnumDisplayMonitors.Call(0, 0, enumMonitor, 0)
	if r == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors: %w", e)
	}
	return append([]Monitor(nil), enumResult...), nil
}

func (win32Desktop) setSpanRegistry() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, desktopRegistryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\%s: %w", desktopRegistryKey, err)
	}
	defer k.Close()
	if err := k.SetStringValue("WallpaperStyle", wallpaperStyleSpanned); err != nil {
		return fmt.Errorf("set WallpaperStyle: %w", err)
	}
	if err := k.SetStringValue("TileWallpaper", "0"); err != nil {
		return fmt.Errorf("set TileWallpaper: %w", err)
	}
	return nil
}

func (win32Desktop) applyWallpaper(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode path: %w", err)
	}
	r, _, e := procSystemParametersInfoW.Call(spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(p)),
		spifUpdateIniFile|spifSendWinIniChange)
	if r == 0 {
		return fmt.Errorf("SystemParametersInfoW failed: %w", e)
	}
	return nil
}

// SetAppUserModelID gives the process an explicit AppUserModelID so the
// taskbar groups and pins the GUI correctly (R4.8).
func SetAppUserModelID(id string) error {
	p, err := windows.UTF16PtrFromString(id)
	if err != nil {
		return fmt.Errorf("encode app id: %w", err)
	}
	hr, _, _ := procSetAppUserModelID.Call(uintptr(unsafe.Pointer(p)))
	if int32(hr) < 0 {
		return fmt.Errorf("SetCurrentProcessExplicitAppUserModelID: HRESULT 0x%08x", uint32(hr))
	}
	return nil
}
