//go:build !linux

package gui

// sendNotification uses Fyne's own notification on platforms whose
// notification centres apply their own timeout.
func sendNotification(u *ui, title, body string) {
	u.app.SendNotification(fyneNotification(title, body))
}
