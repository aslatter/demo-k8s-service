package logutil

import (
	"context"
	"log/slog"
)

// WithLogger stores a logger in context for use down-stream.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if _, ok := l.Handler().(*contextHandler); ok {
		// refuse to install a context-handler-logger in context.
		// someone could wrap it in a logging-adapter - there's not
		// much we can do about that.
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, l)
}

// WithLogArgs stores log-arguments in context for use down-stream.
func WithLogArgs(ctx context.Context, args ...any) context.Context {
	// if we have a logger in context, update it
	l, _ := ctx.Value(contextKey{}).(*slog.Logger)
	if l != nil {
		newLogger := l.With(args...)
		return WithLogger(ctx, newLogger)
	}

	// otherwise, check to see if we have a default-logger?
	defaultLogger := slog.Default()

	// was the default logger a contextLogger? if so we don't
	// want to stuff it in context - that would be bad.
	defaultH, _ := defaultLogger.Handler().(*contextHandler)
	if defaultH != nil {
		fallback := defaultH.fallback
		newLogger := slog.New(fallback).With(args...)
		return WithLogger(ctx, newLogger)
	}

	return WithLogger(ctx, defaultLogger.With(args...))
}

// Logger retreives a logger from context.
func Logger(ctx context.Context) *slog.Logger {
	l, _ := ctx.Value(contextKey{}).(*slog.Logger)
	if l == nil {
		l = slog.Default()
	}
	return l
}

func handlerFromContext(ctx context.Context) slog.Handler {
	l, _ := ctx.Value(contextKey{}).(*slog.Logger)
	if l == nil {
		return nil
	}
	return l.Handler()
}

type contextKey struct{}

// SetDefault sets the [slog] default-logger use whichever logger
// has been stored in context, with a fall-back to the passed-in
// logger.
//
// The expected use is:
//
//	myLogger := newSpecialLogger(...)
//	logutil.SetDefault(myLogger)
//
//	ctx := context.Background()
//	ctx = logutil.WithLogArgs(ctx, slog.String("requestId", requestId))
//
//	// logs with the registered logger, but with the additional
//	// attributes in context.
//	slog.WarnContext(ctx, "did a thing")
//
// Or directly override the logger in context, after calling [SetDefault]:
//
//	myOtherLogger := newOtherLogger(...)
//	ctx := logutil.WithLogger(ctx, myOtherLogger)
//
//	// logs with [myOtherLogger]
//	slog.WarnContext(ctx, "did another thing")
func SetDefault(fallback *slog.Logger) {
	slog.SetDefault(slog.New(newContextHandler(fallback.Handler())))
}

type contextHandler struct {
	fallback   slog.Handler
	logDetails *logDetails
}

func newContextHandler(inner slog.Handler) *contextHandler {
	return &contextHandler{fallback: inner}
}

func (c *contextHandler) handler(ctx context.Context) slog.Handler {
	h := handlerFromContext(ctx)
	if h == nil {
		return c.fallback
	}
	return h
}

// Enabled implements slog.Handler.
func (c *contextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return c.handler(ctx).Enabled(ctx, l)
}

// Handle implements slog.Handler.
func (c *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if c.logDetails == nil {
		return c.handler(ctx).Handle(ctx, r)
	}

	// materialize any 'WithAttrs' or 'WithGroup' calls
	// into a new Record.

	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	d := c.logDetails
	for d != nil {
		if d.groupName != "" {
			attrs = []slog.Attr{slog.GroupAttrs(d.groupName, attrs...)}
		} else {
			attrs = append(attrs, d.attrs...)
		}

		d = d.prev
	}

	newR := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	newR.AddAttrs(attrs...)

	return c.handler(ctx).Handle(ctx, newR)
}

// WithAttrs implements slog.Handler.
func (c *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newH := *c
	newH.logDetails = newH.logDetails.withAttrs(attrs)
	return &newH
}

// WithGroup implements slog.Handler.
func (c *contextHandler) WithGroup(name string) slog.Handler {
	newH := *c
	newH.logDetails = newH.logDetails.withGroup(name)
	return &newH
}

type logDetails struct {
	prev      *logDetails
	groupName string
	attrs     []slog.Attr
}

func (d *logDetails) withGroup(groupName string) *logDetails {
	if groupName == "" {
		return d
	}
	return &logDetails{
		prev:      d,
		groupName: groupName,
	}
}

func (d *logDetails) withAttrs(attrs []slog.Attr) *logDetails {
	if len(attrs) == 0 {
		return d
	}
	return &logDetails{
		prev:  d,
		attrs: attrs,
	}
}
