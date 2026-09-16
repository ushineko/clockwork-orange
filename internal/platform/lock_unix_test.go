//go:build unix

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecondTryLockInSameProcessIsRefusedUntilTheFirstIsReleased(t *testing.T) {
	old := lockDir
	lockDir = t.TempDir()
	t.Cleanup(func() { lockDir = old })
	id := fmt.Sprintf("clockwork_platform_test_%d", os.Getpid())

	first, ok := TryLock(id)
	require.True(t, ok)
	require.NotNil(t, first)
	require.FileExists(t, filepath.Join(lockDir, id+".lock"))

	second, ok := TryLock(id)
	require.False(t, ok, "flock is per open file description, so a second opener must be refused")
	require.Nil(t, second)

	first.Release()
	third, ok := TryLock(id)
	require.True(t, ok, "after Release the next TryLock must succeed")
	third.Release()
	third.Release() // idempotent
}

func TestDifferentIDsDoNotContend(t *testing.T) {
	old := lockDir
	lockDir = t.TempDir()
	t.Cleanup(func() { lockDir = old })
	gui, ok := TryLock("clockwork_orange_gui_lock")
	require.True(t, ok)
	defer gui.Release()
	svc, ok := TryLock("clockwork_orange_service_lock")
	require.True(t, ok, "DV10: the GUI and daemon locks are independent")
	svc.Release()
}

func TestTryLockFailsOpenWhenTheLockFileCannotBeCreated(t *testing.T) {
	old := lockDir
	lockDir = filepath.Join(t.TempDir(), "does", "not", "exist")
	t.Cleanup(func() { lockDir = old })
	l, ok := TryLock("x")
	require.True(t, ok, "an unusable lock directory must not stop the program")
	require.NotNil(t, l)
	l.Release()
}
