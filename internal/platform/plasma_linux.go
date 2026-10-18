//go:build linux

package platform

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/godbus/dbus/v5"
)

/*
Plasma is the part of plasmashell's D-Bus interface the desktop needs (spec
028): one screen's wallpaper, set and read, the signal that one changed, and
the screen locker's reload. Screens are Plasma's screen numbers, so desktop
containments Plasma keeps for disconnected screens never take part (#47).

It replaces the qdbus6 process and the evaluateScript JavaScript that 2.9.5
used (DV18): no script means no path to escape into one.
*/
type Plasma interface {
	// SetImage sets screen's org.kde.image wallpaper. A screen that does not
	// exist is ignored by Plasma; MonitorCount is the guard.
	SetImage(ctx context.Context, screen int, path string) error
	// Image is screen's image path. ok is false when the screen does not
	// exist (Plasma answers with an empty map) or shows another plugin.
	Image(ctx context.Context, screen int) (path string, exists bool, err error)
	// Changes reports the screen of every wallpaperChanged signal until ctx
	// ends.
	Changes(ctx context.Context) (<-chan int, error)
	// ReloadScreenSaver asks the screen locker to re-read its config.
	ReloadScreenSaver(ctx context.Context) error
}

const (
	plasmaDest  = "org.kde.plasmashell"
	plasmaPath  = "/PlasmaShell"
	plasmaIface = "org.kde.PlasmaShell"
	imagePlugin = "org.kde.image"
)

// busPlasma is Plasma over the session bus. godbus shares one connection per
// process; it is not closed here.
type busPlasma struct{}

func (busPlasma) shell() (dbus.BusObject, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	return conn.Object(plasmaDest, plasmaPath), nil
}

func (b busPlasma) SetImage(ctx context.Context, screen int, path string) error {
	obj, err := b.shell()
	if err != nil {
		return err
	}
	params := map[string]dbus.Variant{"Image": dbus.MakeVariant("file://" + path)}
	if err := obj.CallWithContext(ctx, plasmaIface+".setWallpaper", 0, imagePlugin, params, uint32(screen)).Err; err != nil { //nolint:gosec // G115: screen is a small non-negative index
		return fmt.Errorf("plasmashell setWallpaper on screen %d: %w", screen, err)
	}
	return nil
}

func (b busPlasma) Image(ctx context.Context, screen int) (string, bool, error) {
	obj, err := b.shell()
	if err != nil {
		return "", false, err
	}
	var m map[string]dbus.Variant
	if err := obj.CallWithContext(ctx, plasmaIface+".wallpaper", 0, uint32(screen)).Store(&m); err != nil { //nolint:gosec // G115: as above
		return "", false, fmt.Errorf("plasmashell wallpaper of screen %d: %w", screen, err)
	}
	if len(m) == 0 {
		return "", false, nil
	}
	if p, ok := m["wallpaperPlugin"].Value().(string); ok && p != imagePlugin {
		return "", true, nil
	}
	img, _ := m["Image"].Value().(string)
	return PathFromImageURL(img), true, nil
}

func (b busPlasma) Changes(ctx context.Context) (<-chan int, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	match := []dbus.MatchOption{
		dbus.WithMatchObjectPath(plasmaPath),
		dbus.WithMatchInterface(plasmaIface),
		dbus.WithMatchMember("wallpaperChanged"),
	}
	if err := conn.AddMatchSignalContext(ctx, match...); err != nil {
		return nil, fmt.Errorf("watch wallpaperChanged: %w", err)
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	out := make(chan int, 16)
	go func() {
		defer func() {
			conn.RemoveSignal(signals)
			_ = conn.RemoveMatchSignal(match...)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case s := <-signals:
				if s == nil || s.Name != plasmaIface+".wallpaperChanged" || len(s.Body) != 1 {
					continue
				}
				if screen, ok := s.Body[0].(uint32); ok {
					select {
					case out <- int(screen):
					default: // a consumer that is behind reads the screen anyway
					}
				}
			}
		}
	}()
	return out, nil
}

func (busPlasma) ReloadScreenSaver(ctx context.Context) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("session bus: %w", err)
	}
	if err := conn.Object("org.freedesktop.ScreenSaver", "/ScreenSaver").
		CallWithContext(ctx, "org.freedesktop.ScreenSaver.configure", 0).Err; err != nil {
		return fmt.Errorf("screen saver configure: %w", err)
	}
	return nil
}

// PathFromImageURL turns Plasma's Image value into a path: this program
// writes file:// and the raw path, KDE's own dialog percent-encodes it.
func PathFromImageURL(s string) string {
	if !strings.HasPrefix(s, "file://") {
		return s
	}
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		return u.Path
	}
	return strings.TrimPrefix(s, "file://")
}
