package gui

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

// restoreMemLimit puts the runtime back as the test found it, because
// SetMemoryLimit is process-wide and a test that leaves it set changes how
// every test after it collects.
func restoreMemLimit(t *testing.T) {
	t.Helper()
	was := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(was) })
}

// With nothing set, the window runs under the default ceiling (spec 018 R1).
func TestTheMemoryCeilingDefaults(t *testing.T) {
	restoreMemLimit(t)
	t.Setenv("GOMEMLIMIT", "")
	t.Setenv(memLimitEnv, "")
	require.Equal(t, int64(defaultMemLimit), setMemLimit())
	require.Equal(t, int64(defaultMemLimit), debug.SetMemoryLimit(-1))
}

// GOMEMLIMIT is the runtime's own switch and wins: a person who sets it means
// it, and overriding it from inside would make the documented variable a lie.
func TestTheRuntimesOwnLimitWins(t *testing.T) {
	restoreMemLimit(t)
	t.Setenv("GOMEMLIMIT", "512MiB")
	t.Setenv(memLimitEnv, "123456789")
	require.Zero(t, setMemLimit(), "the program does not argue with GOMEMLIMIT")
}

// The program's own variable overrides the default, and turns the ceiling off
// when asked (R2).
func TestTheCeilingCanBeSetOrTurnedOff(t *testing.T) {
	restoreMemLimit(t)
	t.Setenv("GOMEMLIMIT", "")

	t.Setenv(memLimitEnv, "268435456")
	require.Equal(t, int64(268435456), setMemLimit())

	for _, off := range []string{"0", "-1"} {
		t.Setenv(memLimitEnv, off)
		require.Zerof(t, setMemLimit(), "%q asks for no ceiling", off)
	}
}

// An unreadable value leaves the runtime alone rather than guessing: a typo in
// a desktop entry should not silently change how the window collects.
func TestAnUnreadableCeilingIsIgnored(t *testing.T) {
	restoreMemLimit(t)
	t.Setenv("GOMEMLIMIT", "")
	t.Setenv(memLimitEnv, "768MiB") // the runtime's spelling, not a byte count
	require.Zero(t, setMemLimit())
}

/*
The preview box is sized so two caches stay well under the ceiling (R3).

At 1600x900 with a cap of sixteen, the review's cache and the run dialog's held
176 MB of a 228 MB live heap -- measured, not estimated. This pins the
arithmetic that replaced it, so a later change to the geometry has to look at
the number again.
*/
func TestTheTwoPreviewCachesFitTheCeiling(t *testing.T) {
	const bytesPerPixel = 4
	frame := previewMaxW * previewMaxH * bytesPerPixel
	both := int64(frame * previewCap * 2)

	require.Less(t, both, int64(100<<20), "two full caches stay under 100 MiB")
	require.Less(t, both*4, int64(defaultMemLimit),
		"and leave the ceiling room for the fonts, the decode in flight and the rest of the window")
}
