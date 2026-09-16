/*
Package events is the reporting surface shared by every long-running operation
(spec 010 R5.1, R6.7).

It is a leaf package so that plugins, the engine and core can all report
through it without an import cycle: core imports plugins, plugins must not
import core.

In the Python implementation plugins wrote human-readable logs to stderr and
two structured markers, `::PROGRESS:: <pct> :: <msg>` and
`::IMAGE_SAVED:: <path>`. Those markers become the typed callbacks below; the
CLI renders them back into the same text lines so scripts that grep for them
keep working.
*/
package events

import "fmt"

// Level ranks a log line so a front end can present it by importance.
type Level int

// Log levels, ordered from least to most important.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String names a level the way the Python implementation printed it:
// bracketed and upper-case, e.g. "[DEBUG]".
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "[DEBUG]"
	case LevelInfo:
		return "[INFO]"
	case LevelWarn:
		return "[WARN]"
	case LevelError:
		return "[ERROR]"
	default:
		return "[?]"
	}
}

/*
Events is how an operation talks back to whichever front end started it.

Every field may be nil and every call site goes through the methods, so a
caller that does not care about progress passes the zero value. That matters
more than it sounds: the alternative -- an interface with a no-op
implementation -- makes every request carry a non-nil field that a zero value
cannot satisfy.
*/
type Events struct {
	// OnLog receives one human-readable line.
	OnLog func(level Level, msg string)
	// OnProgress receives a 0-100 percentage and a short message.
	OnProgress func(pct int, msg string)
	// OnImageSaved receives the absolute path of a newly downloaded image.
	OnImageSaved func(path string)
}

// Log emits one line at the given level.
func (e Events) Log(level Level, msg string) {
	if e.OnLog != nil {
		e.OnLog(level, msg)
	}
}

// Logf emits one formatted line at the given level.
func (e Events) Logf(level Level, format string, args ...any) {
	if e.OnLog != nil {
		e.OnLog(level, fmt.Sprintf(format, args...))
	}
}

// Debugf emits at LevelDebug.
func (e Events) Debugf(format string, args ...any) { e.Logf(LevelDebug, format, args...) }

// Infof emits at LevelInfo.
func (e Events) Infof(format string, args ...any) { e.Logf(LevelInfo, format, args...) }

// Warnf emits at LevelWarn.
func (e Events) Warnf(format string, args ...any) { e.Logf(LevelWarn, format, args...) }

// Errorf emits at LevelError.
func (e Events) Errorf(format string, args ...any) { e.Logf(LevelError, format, args...) }

// Progress reports how far through a long operation we are.
func (e Events) Progress(pct int, msg string) {
	if e.OnProgress != nil {
		e.OnProgress(pct, msg)
	}
}

// ImageSaved reports a newly saved image.
func (e Events) ImageSaved(path string) {
	if e.OnImageSaved != nil {
		e.OnImageSaved(path)
	}
}
