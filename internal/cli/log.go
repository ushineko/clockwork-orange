package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/ushineko/clockwork-orange/internal/events"
)

/*
bracketHandler is a log/slog handler that prints one line per record in the
form the Python printed: `[DEBUG] message` (R6.7). No timestamps and no
key=value attributes, because the systemd journal adds its own timestamp and
the lines are read by people grepping `journalctl`, not by a log pipeline.
Every line is written with one call so concurrent plugin goroutines cannot
interleave halves of two lines.
*/
type bracketHandler struct {
	w   io.Writer
	min slog.Level
	mu  *sync.Mutex
}

func (h *bracketHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }

func (h *bracketHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintf(h.w, "%s %s\n", levelWord(r.Level), r.Message)
	return err //nolint:wrapcheck // an unwritable stderr has no useful context to add
}

// WithAttrs and WithGroup return the handler unchanged: attributes are not
// rendered (see the type comment).
func (h *bracketHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *bracketHandler) WithGroup(string) slog.Handler      { return h }

// levelWord is the bracketed prefix for a slog level.
func levelWord(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return events.LevelError.String()
	case l >= slog.LevelWarn:
		return events.LevelWarn.String()
	case l >= slog.LevelInfo:
		return events.LevelInfo.String()
	default:
		return events.LevelDebug.String()
	}
}

func toSlog(l events.Level) slog.Level {
	switch l {
	case events.LevelError:
		return slog.LevelError
	case events.LevelWarn:
		return slog.LevelWarn
	case events.LevelInfo:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

/*
newEvents routes core's log lines to w through slog and renders progress and
saved-image callbacks as the `::PROGRESS:: <pct> :: <msg>` and
`::IMAGE_SAVED:: <path>` marker lines the Python plugins printed (R6.7), so a
script that grepped for them keeps working. Markers ignore --log-level: they
are protocol, not chatter.
*/
func newEvents(w io.Writer, minLevel events.Level) events.Events {
	h := &bracketHandler{w: w, min: toSlog(minLevel), mu: &sync.Mutex{}}
	logger := slog.New(h)
	marker := func(format string, args ...any) {
		h.mu.Lock()
		defer h.mu.Unlock()
		_, _ = fmt.Fprintf(w, format+"\n", args...)
	}
	return events.Events{
		OnLog: func(level events.Level, msg string) {
			logger.Log(context.Background(), toSlog(level), msg)
		},
		OnProgress:   func(pct int, msg string) { marker("::PROGRESS:: %d :: %s", pct, msg) },
		OnImageSaved: func(path string) { marker("::IMAGE_SAVED:: %s", path) },
	}
}
