package gui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"
)

/*
An opt-in profiling server, for diagnosing this window's own resource use
(spec 017).

Off unless CLOCKWORK_PPROF names an address, and bound to the loopback
interface whatever that address says, because a profiling endpoint exposes the
program's memory and goroutine state and has no business on a network
interface. The window is a desktop program on a machine with a browser: the
address is for the person sitting at it.

	CLOCKWORK_PPROF=:6060 clockwork-orange-gui
	go tool pprof http://127.0.0.1:6060/debug/pprof/heap

It exists because the alternative is inference. A report of memory climbing
during a download was answered twice from reading the code, and the second
reading was wrong; a heap profile would have said so in a minute.
*/
const pprofEnv = "CLOCKWORK_PPROF"

// pprofTimeouts keep a stuck client from holding the listener open. A profile
// can legitimately take thirty seconds, hence the generous write timeout.
const (
	pprofReadTimeout   = 10 * time.Second
	pprofWriteTimeout  = 2 * time.Minute
	pprofListenTimeout = 5 * time.Second
)

/*
startPprof starts the profiling server when the environment asks for one, and
returns a function that stops it. It returns a nil-safe no-op otherwise, so
the caller needs no branch.

Failures are reported and not fatal: a profiling server that cannot bind is a
reason to carry on without one, never a reason for the window not to open.
*/
func startPprof(report func(string)) func() {
	addr := os.Getenv(pprofEnv)
	if addr == "" {
		return func() {}
	}
	addr = loopback(addr)

	mux := http.NewServeMux()
	// Registered by hand rather than by importing net/http/pprof for its
	// side effect on DefaultServeMux: this program should not be one import
	// away from serving profiles from a mux something else might also use.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// ListenConfig rather than net.Listen: the linter wants a context on the
	// listen, and a profiling server that cannot bind promptly is one the
	// window should carry on without.
	lc := net.ListenConfig{}
	ctx, cancel := context.WithTimeout(context.Background(), pprofListenTimeout)
	defer cancel()
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		report(fmt.Sprintf("Profiling server on %s: %v", addr, err))
		return func() {}
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: pprofReadTimeout, WriteTimeout: pprofWriteTimeout}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			report(fmt.Sprintf("Profiling server: %v", err))
		}
	}()
	report(fmt.Sprintf("Profiling on http://%s/debug/pprof/", ln.Addr()))
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

/*
loopback forces addr onto the loopback interface.

A bare port (":6060") means every interface to net.Listen, which is how a
debugging endpoint ends up reachable from the network. Whatever host the
caller gave, the listener is 127.0.0.1; a port alone is honoured, a host is
replaced.
*/
func loopback(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Not host:port -- treat the whole thing as a port, so "6060" works.
		return net.JoinHostPort("127.0.0.1", addr)
	}
	return net.JoinHostPort("127.0.0.1", port)
}
