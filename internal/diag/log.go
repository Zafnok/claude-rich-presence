package diag

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
)

// Logger writes log lines. A nil Logger discards them. It is safe for use by
// several goroutines.
//
// A failure to write is ignored: logging never stops the program.
type Logger struct {
	handler slog.Handler
	now     func() time.Time
}

// NewLogger returns a logger that writes lines at level and above to w, one
// Write per line, stamped with the time now returns.
func NewLogger(w io.Writer, level config.LogLevel, now func() time.Time) *Logger {
	return &Logger{
		handler: slog.NewTextHandler(w, &slog.HandlerOptions{Level: slogLevel(level)}),
		now:     now,
	}
}

// Open returns a logger that writes to the log file in dir, which is
// config.Dirs.Logs. If dir is empty or cannot be created, the logger
// discards everything.
func Open(fsys FS, dir string, level config.LogLevel, now func() time.Time) *Logger {
	return NewLogger(NewFile(fsys, dir, MaxLogSize), level, now)
}

// slogLevel maps a configured level to the standard library's. Anything
// unknown is the default, warning.
func slogLevel(level config.LogLevel) slog.Level {
	switch level {
	case config.LogError:
		return slog.LevelError
	case config.LogInfo:
		return slog.LevelInfo
	case config.LogDebug:
		return slog.LevelDebug
	}
	return slog.LevelWarn
}

// Error logs something that stopped presence from working.
func (l *Logger) Error(msg text, attrs ...Attr) { l.log(slog.LevelError, msg, attrs) }

// Warn logs something that went wrong and was recovered from.
func (l *Logger) Warn(msg text, attrs ...Attr) { l.log(slog.LevelWarn, msg, attrs) }

// Info logs a normal change of state.
func (l *Logger) Info(msg text, attrs ...Attr) { l.log(slog.LevelInfo, msg, attrs) }

// Debug logs detail for someone following the program's steps.
func (l *Logger) Debug(msg text, attrs ...Attr) { l.log(slog.LevelDebug, msg, attrs) }

func (l *Logger) log(level slog.Level, msg text, attrs []Attr) {
	ctx := context.Background()
	if l == nil || !l.handler.Enabled(ctx, level) {
		return
	}
	r := slog.NewRecord(l.now(), level, string(msg), 0)
	for _, a := range attrs {
		r.AddAttrs(a.attr)
	}
	_ = l.handler.Handle(ctx, r)
}
