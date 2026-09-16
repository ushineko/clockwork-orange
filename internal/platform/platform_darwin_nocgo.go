//go:build darwin && !cgo

package platform

// nativeScreenCount has no AppKit in a CGO_ENABLED=0 build; MonitorCount
// falls through to osascript.
func (*DarwinPlatform) nativeScreenCount() (int, bool) { return 0, false }

// setDesktopImagesNative reports that AppKit is unavailable so setImages
// uses osascript.
func (*DarwinPlatform) setDesktopImagesNative([]string) error { return errNoAppKit }
