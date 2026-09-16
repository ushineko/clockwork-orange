//go:build windows

package gui

// listenShow and requestShow are the Unix second-launch hand-off (R7.19);
// on Windows the second instance still exits silently. Phase 6 revisits this
// with a named pipe once the Windows build is exercised.
func (u *ui) listenShow() func() { return func() {} }

func requestShow() bool { return false }
