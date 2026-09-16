//go:build darwin && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework Foundation
#import <AppKit/AppKit.h>
#include <stdio.h>
#include <stdlib.h>

static int co_screen_count(void) {
	@autoreleasepool {
		return (int)[[NSScreen screens] count];
	}
}

// co_set_desktop_images assigns paths[i % n] to NSScreen i with the scaling
// options the Python used (R4.9). An empty path skips that screen. Returns 0
// on success, -1 when n <= 0, otherwise 1 + the index of the screen that
// failed, with the NSError description copied into errbuf.
static int co_set_desktop_images(const char **paths, int n, char *errbuf, int errlen) {
	@autoreleasepool {
		if (n <= 0) {
			return -1;
		}
		NSWorkspace *ws = [NSWorkspace sharedWorkspace];
		NSArray<NSScreen *> *screens = [NSScreen screens];
		NSDictionary<NSWorkspaceDesktopImageOptionKey, id> *opts = @{
			NSWorkspaceDesktopImageScalingKey : @(NSImageScaleProportionallyUpOrDown),
			NSWorkspaceDesktopImageAllowClippingKey : @YES,
		};
		for (NSUInteger i = 0; i < [screens count]; i++) {
			const char *p = paths[i % (NSUInteger)n];
			if (p == NULL || p[0] == '\0') {
				continue;
			}
			NSString *path = [NSString stringWithUTF8String:p];
			NSURL *url = [NSURL fileURLWithPath:path];
			NSError *err = nil;
			BOOL ok = [ws setDesktopImageURL:url forScreen:screens[i] options:opts error:&err];
			if (!ok) {
				const char *desc = err ? [[err localizedDescription] UTF8String] : "unknown error";
				snprintf(errbuf, (size_t)errlen, "%s", desc ? desc : "unknown error");
				return (int)i + 1;
			}
		}
		return 0;
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// nativeScreenCount is NSScreen.screens.count (R4.9).
func (*DarwinPlatform) nativeScreenCount() (int, bool) {
	n := int(C.co_screen_count())
	return n, n > 0
}

// setDesktopImagesNative calls NSWorkspace
// setDesktopImageURL:forScreen:options:error: per screen (D11, R4.9).
func (*DarwinPlatform) setDesktopImagesNative(paths []string) error {
	if len(paths) == 0 {
		return errors.New("no image paths provided")
	}
	cpaths := make([]*C.char, len(paths))
	for i, s := range paths {
		cpaths[i] = C.CString(s)
	}
	defer func() {
		for _, cp := range cpaths {
			C.free(unsafe.Pointer(cp))
		}
	}()
	errbuf := make([]C.char, 1024)
	rc := C.co_set_desktop_images((**C.char)(unsafe.Pointer(&cpaths[0])), C.int(len(paths)),
		&errbuf[0], C.int(len(errbuf)))
	switch {
	case rc == 0:
		return nil
	case rc < 0:
		return errors.New("no image paths provided")
	default:
		return fmt.Errorf("failed to set wallpaper on screen %d: %s", int(rc)-1, C.GoString(&errbuf[0]))
	}
}
