package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/aslatter/demo-k8s-service/internal/httputil"
	"github.com/aslatter/demo-k8s-service/internal/lifecycle"
	"github.com/aslatter/demo-k8s-service/internal/logutil"
	"github.com/aslatter/go-router"
	"golang.org/x/sys/unix"
)

func main() {
	if err := mainErr(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func mainErr() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)
	defer cancel()

	logCfg := logutil.DefaultConfig()
	logCfg.AddToFlagSet(flag.CommandLine)

	httpCfg := httputil.DefaultConfig()
	httpCfg.AddToFlagSet(flag.CommandLine)

	flag.Parse()

	logger := logutil.FromConfig(logCfg)
	logutil.SetDefault(logger)
	ctx = logutil.WithLogger(ctx, logger)

	root := router.New()

	probes := root.New("/-/")
	probes.Use(httputil.LoggingMiddleware(true))

	apiRouter := root.New("")
	apiRouter.Use(httputil.LoggingMiddleware(false))

	remainder := root.New("") // mainly to catch 404s
	remainder.Use(httputil.LoggingMiddleware(true))
	remainder.HandleFunc("/", http.NotFound)

	// application routes go here
	apiRouter.HandleFunc("/echo/", func(w http.ResponseWriter, r *http.Request) {
		// ok
	})

	probes.HandleFunc("GET /livez", func(w http.ResponseWriter, r *http.Request) {
		// ok
	})
	probes.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		// ok
	})

	httpServer := httputil.FromConfig(httpCfg)
	httpServer.Handler = root.Handler()

	var c lifecycle.Container

	c.Components = append(c.Components, httpServer.Run)

	return c.Run(ctx)
}
