//go:build unix

package platform

import (
	"errors"
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// lockDir is where TryLock creates its lock files; a variable so tests can
// keep their locks out of the shared /tmp.
var lockDir = "/tmp"

type flockLock struct{ f *os.File }

// Release implements Lock: unlock and close. The file is left in place, as
// the Python did; removing it would race a concurrent starter that already
// opened it.
func (l *flockLock) Release() {
	if l.f == nil {
		return
	}
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}

type noopLock struct{}

func (noopLock) Release() {}

// TryLock takes an exclusive, non-blocking flock on /tmp/<id>.lock (R4.11,
// DV10). false means another process holds it. The lock lives as long as the
// returned Lock is not released and the process is alive; flock is per open
// file description, so a second TryLock in the same process also gets false.
//
// A failure to create or lock the file for any reason other than contention
// logs and fails open (true), so a read-only /tmp cannot stop the program
// from starting. (The Python returned False there, which would have made the
// GUI silently refuse to start.)
func TryLock(id string) (Lock, bool) {
	path := lockDir + "/" + id + ".lock"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302: the file carries no data; it is a lock token, 0644 as in the Python.
	if err != nil {
		log.Printf("[WARN] Could not create lock file %s: %v (continuing without single-instance lock)", path, err)
		return noopLock{}, true
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, false
		}
		log.Printf("[WARN] Could not lock %s: %v (continuing without single-instance lock)", path, err)
		return noopLock{}, true
	}
	return &flockLock{f: f}, true
}
