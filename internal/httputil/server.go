package httputil

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aslatter/demo-k8s-service/internal/logutil"
)

type Server struct {
	Name                  string
	Address               string
	CertFile              string
	KeyFile               string
	Handler               http.Handler
	PreShutdownDelay      time.Duration
	ShutdownDrainDuration time.Duration
}

func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	name := s.Name
	if name == "" {
		name = "http"
	}

	ctx = logutil.WithLogArgs(ctx,
		slog.String("component", name),
	)

	requestContext := context.WithoutCancel(ctx)

	h := &http.Server{
		Handler: s.Handler,
		BaseContext: func(_ net.Listener) context.Context {
			return requestContext
		},
	}

	var wg sync.WaitGroup

	var listener net.Listener
	if s.CertFile != "" {
		watcher, err := newCertWatcher(s.CertFile, s.KeyFile)
		if err != nil {
			return fmt.Errorf("creating cert watcher: %s", err)
		}
		wg.Go(func() {
			err := watcher.run(ctx)
			if err == nil || errors.Is(err, context.Canceled) {
				return
			}
			// shut down server
			slog.ErrorContext(ctx, "watching cert-files", slog.Any("err", err))
			cancel()
		})
		listener, err = tls.Listen("tcp", s.Address, &tls.Config{
			GetCertificate: watcher.getCertificate,
		})
		if err != nil {
			return fmt.Errorf("starting tls listener: %s", err)
		}
	} else {
		var err error
		listener, err = net.Listen("tcp", s.Address)
		if err != nil {
			return err
		}
	}

	s.Address = listener.Addr().String()
	slog.InfoContext(ctx, "serving", "addr", s.Address)

	wg.Go(func() {
		<-ctx.Done()
		slog.InfoContext(ctx, "shutting down")
		slog.InfoContext(ctx, "waiting before we stop accepting new connections")
		time.Sleep(s.PreShutdownDelay)

		shutdownCtx := context.WithoutCancel(ctx)
		shutdownCtx, shutdownCancel := context.WithTimeout(shutdownCtx, s.ShutdownDrainDuration)
		defer shutdownCancel()

		slog.InfoContext(ctx, "draining active connections")
		_ = h.Shutdown(shutdownCtx)
		slog.InfoContext(ctx, "closing active connections")
		_ = h.Close()
	})

	err := h.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		slog.ErrorContext(ctx, "stopped serving", slog.Any("err", err))
	}
	cancel()

	wg.Wait()
	return err
}
