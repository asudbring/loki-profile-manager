package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/asudbring/loki-profile-manager/internal/config"
)

type Options struct {
	Verbose        bool
	TerminalWriter io.Writer
	Redactor       *Redactor
}

type Logger struct {
	logger *slog.Logger
	file   *os.File
}

func NewLogger(paths config.LocalPaths, opts Options) (*Logger, error) {
	if err := os.MkdirAll(paths.LogDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(paths.LogPath), 0o700); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(paths.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	redactor := opts.Redactor
	if redactor == nil {
		redactor = NewRedactor()
	}

	handlers := []slog.Handler{
		newRedactHandler(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelDebug}), redactor),
	}
	if opts.Verbose {
		writer := opts.TerminalWriter
		if writer == nil {
			writer = os.Stderr
		}
		handlers = append(handlers, newRedactHandler(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelDebug}), redactor))
	}

	return &Logger{
		logger: slog.New(multiHandler{handlers: handlers}),
		file:   file,
	}, nil
}

func (l *Logger) Slog() *slog.Logger {
	if l == nil || l.logger == nil {
		return slog.Default()
	}
	return l.logger
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

type redactHandler struct {
	next     slog.Handler
	redactor *Redactor
}

func newRedactHandler(next slog.Handler, redactor *Redactor) slog.Handler {
	return redactHandler{next: next, redactor: redactor}
}

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, record slog.Record) error {
	cloned := slog.NewRecord(record.Time, record.Level, h.redactor.RedactString(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		cloned.AddAttrs(h.redactor.RedactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, cloned)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactHandler{next: h.next.WithAttrs(h.redactor.RedactAttrs(attrs)), redactor: h.redactor}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{next: h.next.WithGroup(name), redactor: h.redactor}
}

type multiHandler struct {
	handlers []slog.Handler
}

func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		out[i] = handler.WithAttrs(attrs)
	}
	return multiHandler{handlers: out}
}

func (h multiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		out[i] = handler.WithGroup(name)
	}
	return multiHandler{handlers: out}
}
