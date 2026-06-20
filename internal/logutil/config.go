package logutil

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/term"
)

// Config represents configuration for logging
type Config struct {
	Level  LogLevel
	Format LogFormat
}

// DefaultConfig returns an appropriate default [Config].
func DefaultConfig() *Config {
	var c Config

	c.Level = LogLevel(slog.LevelInfo)

	c.Format = LogFormatJSON
	if term.IsTerminal(int(os.Stderr.Fd())) {
		c.Format = LogFormatColor
	}

	return &c
}

// AddToFlagSet adds the various properties of the [Config] to a [flag.FlagSet], for
// processing configuration items from the command-line.
func (c *Config) AddToFlagSet(f *flag.FlagSet) {
	f.Var(&c.Level, "log-level", "minimum logging-level")
	f.Var(&c.Format, "log-format", "output-format for logs")
}

type LogLevel slog.Level

func (l *LogLevel) String() string {
	return (*slog.Level)(l).String()
}

func (l *LogLevel) Set(value string) error {
	return (*slog.Level)(l).UnmarshalText([]byte(value))
}

func (l LogLevel) Level() slog.Level {
	return slog.Level(l)
}

type LogFormat int

const (
	LogFormatText LogFormat = iota
	LogFormatJSON
	LogFormatColor
)

func (f LogFormat) String() string {
	switch f {
	case LogFormatText:
		return "text"
	case LogFormatJSON:
		return "json"
	case LogFormatColor:
		return "color"
	default:
		return "UNKNOWN"
	}
}

func (f *LogFormat) Set(value string) error {
	switch value {
	case "text":
		*f = LogFormatText
	case "json":
		*f = LogFormatJSON
	case "color":
		*f = LogFormatColor
	default:
		return fmt.Errorf("unknown log-format %q. Allowed values are 'text', 'json', and 'color", value)
	}
	return nil
}

func FromConfig(logCfg *Config) *slog.Logger {
	var h slog.Handler
	handlerOpts := &slog.HandlerOptions{
		Level: logCfg.Level,
	}
	switch logCfg.Format {
	case LogFormatText:
		h = slog.NewTextHandler(os.Stderr, handlerOpts)
	case LogFormatColor:
		h = NewColorHandler(os.Stderr, handlerOpts)
	default:
		h = slog.NewJSONHandler(os.Stderr, handlerOpts)
	}
	return slog.New(h)
}
