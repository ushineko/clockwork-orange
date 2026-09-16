//go:build windows

package platform

import (
	"errors"
	"log"

	"golang.org/x/sys/windows"
)

type mutexLock struct{ h windows.Handle }

// Release implements Lock. Closing the last handle destroys a named mutex.
func (l *mutexLock) Release() {
	if l.h != 0 {
		_ = windows.ReleaseMutex(l.h)
		_ = windows.CloseHandle(l.h)
		l.h = 0
	}
}

type noopLock struct{}

func (noopLock) Release() {}

// TryLock creates the named mutex Global\<id> (R4.8). ERROR_ALREADY_EXISTS
// means another instance owns it: false. A failure to create the mutex at
// all logs and fails open (true), exactly as the Python did.
func TryLock(id string) (Lock, bool) {
	name, err := windows.UTF16PtrFromString(`Global\` + id)
	if err != nil {
		log.Printf("[WARN] Invalid mutex name %q: %v (continuing without single-instance lock)", id, err)
		return noopLock{}, true
	}
	h, err := windows.CreateMutex(nil, true, name)
	if h == 0 {
		log.Printf("[ERROR] CreateMutexW failed. Error: %v (continuing without single-instance lock)", err)
		return noopLock{}, true
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(h)
		return nil, false
	}
	return &mutexLock{h: h}, true
}
