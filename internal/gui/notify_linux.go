//go:build linux

package gui

import (
	"github.com/godbus/dbus/v5"
)

// notifyExpiry is how long a desktop notification stays up (R7.11). The
// Python's tray messages used 1–3 s; Fyne's own SendNotification passes 0,
// which on KDE means "until dismissed", so the popups piled up.
const notifyExpiryMs = 3000

/*
sendNotification posts to org.freedesktop.Notifications directly, with the
expiry Fyne does not set and the theme icon the packages install. Any failure
falls back to Fyne's call, which at worst leaves a persistent popup rather
than none.
*/
func sendNotification(u *ui, title, body string) {
	conn, err := dbus.SessionBus()
	if err != nil {
		u.app.SendNotification(fyneNotification(title, body))
		return
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.Call("org.freedesktop.Notifications.Notify", 0,
		"Clockwork Orange", uint32(0), "clockwork-orange", title, body,
		[]string{}, map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant("io.ushineko.clockwork-orange")},
		int32(notifyExpiryMs))
	if call.Err != nil {
		u.app.SendNotification(fyneNotification(title, body))
	}
}
