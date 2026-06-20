package httputil

import (
	"flag"
	"time"
)

// Config represents a configuration baseline for any HTTP-based
// service.
type Config struct {
	Name                  string
	Address               string
	CertFile              string
	KeyFile               string
	PreShutdownDelay      Duration
	ShutdownDrainDuration Duration
}

// DefaultConfig returns an appropriate default [Config].
func DefaultConfig() *Config {
	var c Config

	c.Address = ":8080"
	c.PreShutdownDelay = Duration(2 * time.Second)
	c.ShutdownDrainDuration = Duration(2 * time.Second)

	return &c
}

// AddToFlagSet adds the various properties of the [Config] to a [flag.FlagSet], for
// processing configuration items from the command-line.
func (c *Config) AddToFlagSet(f *flag.FlagSet) {
	f.StringVar(&c.Address, "http-address", c.Address, "address to listen on")
	f.StringVar(&c.CertFile, "tls-cert-file", "", "server TLS certificate file-path")
	f.StringVar(&c.KeyFile, "tls-key-file", "", "server TLS key file-path")
	f.Var(&c.PreShutdownDelay, "http-pre-shutdown-delay", "duration to wait before shutting down http server")
	f.Var(&c.ShutdownDrainDuration, "http-shutdown-drain-duration", "length of time to wait to drain existing connections")
}

type Duration time.Duration

func (d *Duration) String() string {
	return (*time.Duration)(d).String()
}

func (d *Duration) Set(value string) error {
	td, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	*d = Duration(td)
	return nil
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func FromConfig(httpCfg *Config) *Server {
	return &Server{
		Name:                  httpCfg.Name,
		Address:               httpCfg.Address,
		CertFile:              httpCfg.CertFile,
		KeyFile:               httpCfg.KeyFile,
		PreShutdownDelay:      httpCfg.PreShutdownDelay.Duration(),
		ShutdownDrainDuration: httpCfg.ShutdownDrainDuration.Duration(),
	}
}
