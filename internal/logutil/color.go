package logutil

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"time"
	"unicode"
)

// ColorHandler is a logger intended for use when running a logged-service
// locally in a developer's terminal. It does not aim to be machine-parsable.
// It is similar to the standard "text" logger, except:
//   - Timestamps are printed as seconds since the logger was created
//   - The log-level, message, and timestamp are not printed with a label
//   - Log-levels are color-coded for severity, and messages logged at 'info'
//     or higher are bolded.
type ColorHandler struct {
	opts  slog.HandlerOptions
	w     io.Writer
	start time.Time
	group string
	attrs []slog.Attr
}

func NewColorHandler(w io.Writer, opts *slog.HandlerOptions) *ColorHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &ColorHandler{
		opts:  *opts,
		w:     w,
		start: time.Now(),
	}
}

func (c *ColorHandler) clone() *ColorHandler {
	h := *c
	h.attrs = slices.Clip(h.attrs)
	return &h
}

// Enabled implements slog.Handler.
func (c *ColorHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= c.opts.Level.Level()
}

// Handle implements slog.Handler.
func (c *ColorHandler) Handle(_ context.Context, r slog.Record) error {
	var b []byte

	// time
	if !r.Time.IsZero() {
		since := time.Since(c.start)
		sinceInt := int(math.Round(since.Seconds()))
		b = append(b, '[')
		b = strconv.AppendInt(b, int64(sinceInt), 10)
		b = append(b, []byte("] ")...)
	}

	// level
	var color string
	switch {
	case r.Level >= slog.LevelInfo && r.Level < slog.LevelWarn:
		color = terminalCyan
	case r.Level >= slog.LevelWarn && r.Level < slog.LevelError:
		color = terminalBoldYellow
	case r.Level >= slog.LevelError:
		color = terminalBoldRed
	default:
		color = ""
	}
	if color != "" {
		b = append(b, []byte(color+r.Level.String()+terminalReset)...)
	} else {
		b = append(b, []byte(r.Level.String())...)
	}

	// message
	if r.Message != "" {
		b = append(b, ' ')
		switch {
		case r.Level >= slog.LevelInfo:
			color = terminalBold
		default:
			color = ""
		}
		if color != "" {
			b = append(b, []byte(color)...)
		}
		b = append(b, escapeMsg(r.Message)...)
		if color != "" {
			b = append(b, []byte(terminalReset)...)
		}
	}

	// handler-attributes
	for _, a := range c.attrs {
		b = writeAttr(b, c.group, a)
	}

	// record-attributes
	r.Attrs(func(a slog.Attr) bool {
		b = writeAttr(b, c.group, a)
		return true
	})

	b = append(b, '\n')

	_, err := io.Copy(c.w, bytes.NewBuffer(b))

	return err
}

// writeAttr writes an attribute key-value-pair to the buffer
// 'b' and returned the updated buffer. 'prefix' represents any
// open groups. 'writeAttr' may be called recursively if the passed
// in attribute is a group.
func writeAttr(b []byte, prefix string, a slog.Attr) []byte {
	k := a.Key
	v := a.Value.Resolve()
	if v.Equal(slog.Value{}) {
		return b
	}

	vk := v.Kind()
	if vk == slog.KindGroup {
		groupPrefix := prefix
		// a group without a key should behave as if it
		// is "flattened" into the current scope.
		if k != "" {
			groupPrefix = groupPrefix + k + "."
		}
		for _, ga := range v.Group() {
			b = writeAttr(b, groupPrefix, ga)
		}
		return b
	}
	if k == "" {
		return b
	}
	b = append(b, ' ')
	b = append(b, escapeStr(prefix+k)...)
	b = append(b, '=')

	var str string
	switch vk {
	case slog.KindString:
		str = v.String()
	case slog.KindTime:
		str = v.Time().Format(time.RFC3339)
	case slog.KindAny:
		any := v.Any()
		if tm, ok := any.(encoding.TextMarshaler); ok {
			textBytes, err := tm.MarshalText()
			if err != nil {
				b = fmt.Appendf(b, "ERR:%s", err)
				return b
			}
			str = string(textBytes)
		} else {
			// the standard 'text handler' would treat a byte-slice as if
			// it were a string at this point.
			str = fmt.Sprintf("%+v", any)
		}
	default:
		str = v.String()
	}

	return append(b, escapeStr(str)...)
}

func escapeMsg(message string) []byte {
	isSimple := true
	for _, r := range message {
		if !unicode.IsPrint(r) || r == '"' || r == '=' || r == '\'' {
			isSimple = false
			break
		}
	}
	if isSimple {
		return []byte(message)
	}
	return strconv.AppendQuote(nil, message)
}

func escapeStr(value string) []byte {
	isSimple := true
	for _, r := range value {
		if !unicode.IsPrint(r) || r == ' ' || r == '"' || r == '=' || r == '\'' {
			isSimple = false
			break
		}
	}
	if isSimple {
		return []byte(value)
	}
	return strconv.AppendQuote(nil, value)
}

// WithAttrs implements slog.Handler.
func (c *ColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	new := c.clone()
	if new.group == "" {
		new.attrs = append(new.attrs, attrs...)
	} else {
		// if we are in a group-scope, edit the attribute-keys in-place to
		// act like they are in a group. We have to be careful, because 'empty'
		// attributes should be discarded, and groups without-keys shouldn't suddenly
		// act like normal groups with keys.
		for _, a := range collectAttrs(make([]slog.Attr, 0, len(attrs)), attrs) {
			new.attrs = append(new.attrs, slog.Attr{
				Key:   new.group + a.Key,
				Value: a.Value,
			})
		}
	}
	return new
}

func collectAttrs(collected []slog.Attr, input []slog.Attr) []slog.Attr {
	for _, a := range input {
		if a.Equal(slog.Attr{}) {
			continue
		}
		if a.Value.Kind() == slog.KindGroup && a.Key == "" {
			collected = collectAttrs(collected, a.Value.Group())
			continue
		}
		if a.Key == "" {
			continue
		}
		collected = append(collected, a)
	}
	return collected
}

// WithGroup implements slog.Handler.
func (c *ColorHandler) WithGroup(name string) slog.Handler {
	new := c.clone()
	new.group = c.group + name + "."
	return new
}

var _ slog.Handler = (*ColorHandler)(nil)

const (
	terminalReset      = "\033[0m"
	terminalBold       = "\033[1m"
	terminalBoldYellow = "\033[93m"
	terminalBoldRed    = "\033[91m"
	terminalCyan       = "\033[0;36m"
)
