package gui

import (
	"os"
	"runtime/debug"
	"strconv"
)

/*
A soft memory ceiling for the window (spec 018).

Go grows its heap to about twice what was live at the last collection and does
not give the arena back, so a burst sets the floor for the rest of the run. A
review of 4K wallpapers is exactly that burst: each image is decoded at full
size, about 33 MB, and discarded -- 17.7 GB allocated over one session on the
machine this was measured on, which grew the arena to 1.9 GB and left it there.
Live heap at the end of it was 228 MB.

GOMEMLIMIT is the runtime's answer: a soft limit it collects harder to stay
under, rather than a cap that fails an allocation. Under it the collector runs
more often during a burst and the arena never reaches the size the burst would
otherwise leave behind.

The limit is soft on purpose. Go will exceed it rather than fail, so a machine
doing something this number did not anticipate degrades into more GC instead of
a crash.
*/

// defaultMemLimit is the ceiling when the environment says nothing: enough for
// the two preview caches, the fonts, the decode in flight and room over the
// top, and far below the 1.9 GB a burst reached without one.
const defaultMemLimit = 768 << 20 // 768 MiB

// memLimitEnv overrides it, in bytes; 0 or a negative value turns the ceiling
// off and leaves the runtime's own behaviour. GOMEMLIMIT itself is read by the
// runtime before this runs and wins, because a person who sets it means it.
const memLimitEnv = "CLOCKWORK_MEMLIMIT"

/*
setMemLimit applies the ceiling and returns what it set, or 0 when it left the
runtime alone.

Called before the window is built. It does nothing when GOMEMLIMIT is already
set, because that is the runtime's own switch and overriding it from inside the
program would make the documented variable a lie.
*/
func setMemLimit() int64 {
	if os.Getenv("GOMEMLIMIT") != "" {
		return 0 // the runtime already took it; do not argue
	}
	limit := int64(defaultMemLimit)
	if v := os.Getenv(memLimitEnv); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0 // unreadable: leave the runtime alone rather than guess
		}
		if n <= 0 {
			return 0 // asked for no ceiling
		}
		limit = n
	}
	debug.SetMemoryLimit(limit)
	return limit
}
