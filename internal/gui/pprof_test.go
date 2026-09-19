package gui

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Off unless asked for, and asking for nothing costs nothing (spec 017 R1).
func TestPprofIsOffUnlessTheEnvironmentAsks(t *testing.T) {
	t.Setenv(pprofEnv, "")
	var said []string
	stop := startPprof(func(m string) { said = append(said, m) })
	require.NotNil(t, stop, "the stopper is safe to call whether or not a server started")
	stop()
	require.Empty(t, said, "a program nobody asked to profile says nothing")
}

/*
Whatever address is asked for, the listener is on the loopback interface
(spec 017 R2).

A profiling endpoint hands out the program's memory and goroutine state. ":0"
means every interface to net.Listen, which is how a debugging endpoint ends up
reachable from the network; this is a desktop program and the address is for
the person sitting at the machine.
*/
func TestPprofBindsToLoopbackWhateverItIsAsked(t *testing.T) {
	for _, asked := range []string{":6060", "0.0.0.0:6060", "6060", "[::]:6060", "192.168.1.5:6060"} {
		host, port, err := net.SplitHostPort(loopback(asked))
		require.NoErrorf(t, err, "asked %q", asked)
		require.Equalf(t, "127.0.0.1", host, "asked %q", asked)
		require.Equalf(t, "6060", port, "asked %q", asked)
	}
}

// It serves the heap profile and stops when told.
func TestPprofServesAndStops(t *testing.T) {
	t.Setenv(pprofEnv, "0.0.0.0:0") // port 0: the kernel picks, and loopback still wins
	var said []string
	stop := startPprof(func(m string) { said = append(said, m) })
	t.Cleanup(stop)
	require.Len(t, said, 1)
	require.Contains(t, said[0], "http://127.0.0.1:")

	url := said[0][len("Profiling on ") : len(said[0])-len("/debug/pprof/")]
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Get(url + "/debug/pprof/heap?debug=1") //nolint:noctx // a loopback URL this test started
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	require.Equal(t, http.StatusOK, res.StatusCode)

	stop()
	after, err := client.Get(url + "/debug/pprof/heap") //nolint:noctx // as above
	if err == nil {
		_ = after.Body.Close()
	}
	require.Error(t, err, "the listener is closed")
}
