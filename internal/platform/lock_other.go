//go:build !unix && !windows

package platform

import "log"

type noopLock struct{}

func (noopLock) Release() {}

// TryLock has no locking primitive on this OS and fails open.
func TryLock(id string) (Lock, bool) {
	log.Printf("[WARN] No single-instance lock available for %s on this OS", id)
	return noopLock{}, true
}
