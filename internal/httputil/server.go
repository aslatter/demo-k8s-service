package httputil

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
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

	var tlsConfig *tls.Config
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
		tlsConfig = &tls.Config{
			GetCertificate: watcher.getCertificate,
		}
	} else {
		cert, err := generateCertificate()
		if err != nil {
			return err
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	listener, err := tls.Listen("tcp", s.Address, tlsConfig)
	if err != nil {
		return fmt.Errorf("starting tls listener: %s", err)
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

	err = h.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		slog.ErrorContext(ctx, "stopped serving", slog.Any("err", err))
	}
	cancel()

	wg.Wait()
	return err
}

func generateCertificate() (tls.Certificate, error) {
	var z tls.Certificate

	now := time.Now()
	cert := &x509.Certificate{
		NotBefore:             now.Add(-10 * time.Minute),
		NotAfter:              now.AddDate(40, 0, 0),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	maxSerial := (&big.Int{}).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, maxSerial)
	if err != nil {
		return z, fmt.Errorf("generating serial number: %s", err)
	}
	cert.SerialNumber = serial

	privateKey, err := rsa.GenerateKey(nil, 2048)
	if err != nil {
		return z, fmt.Errorf("generating private key: %s", err)
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &privateKey.PublicKey, privateKey)
	if err != nil {
		return z, fmt.Errorf("generating certificate: %s", err)
	}
	var certPemBytes bytes.Buffer
	err = pem.Encode(&certPemBytes, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		return z, fmt.Errorf("encoding certificate: %s", err)
	}

	var keyPemBytes bytes.Buffer
	err = pem.Encode(&keyPemBytes, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err != nil {
		return z, fmt.Errorf("encoding private key: %s", err)
	}

	return tls.X509KeyPair(certPemBytes.Bytes(), keyPemBytes.Bytes())

}
