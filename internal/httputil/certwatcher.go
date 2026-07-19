package httputil

import (
	"context"
	"crypto/tls"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/blake2b"
)

type certWatcher struct {
	certPath string
	keyPath  string
	certHash [32]byte
	keyHash  [32]byte
	cert     atomic.Pointer[tls.Certificate]
}

func newCertWatcher(certPath string, keyPath string) (*certWatcher, error) {

	cw := &certWatcher{
		certPath: certPath,
		keyPath:  keyPath,
	}

	// load the cert sync the first time to fail-fast (and make the struct
	// ready to serve immediately)
	err := cw.reloadCerts(nil)
	if err != nil {
		return nil, err
	}

	return cw, nil
}

func (cw *certWatcher) run(ctx context.Context) error {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}

		err := cw.reloadCerts(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "reloading certs", slog.Any("err", err))
		}
	}
}

func (cw *certWatcher) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return cw.cert.Load(), nil
}

type certFileContents struct {
	certBytes []byte
	keyBytes  []byte
	certHash  [32]byte
	keyHash   [32]byte
}

func (cw *certWatcher) reloadCerts(ctx context.Context) error {
	c, err := cw.loadFileContents()
	if err != nil {
		return err
	}
	if cw.certHash == c.certHash && cw.keyHash == c.keyHash {
		return nil
	}

	if ctx != nil {
		slog.InfoContext(ctx, "reloading certs")
	}

	cert, err := tls.X509KeyPair(c.certBytes, c.keyBytes)
	if err != nil {
		return err
	}
	cw.cert.Store(&cert)
	cw.certHash = c.certHash
	cw.keyHash = c.keyHash

	return nil
}

func (cw *certWatcher) loadFileContents() (ret certFileContents, err error) {
	ret.certBytes, err = os.ReadFile(cw.certPath)
	if err != nil {
		return certFileContents{}, err
	}
	ret.keyBytes, err = os.ReadFile(cw.keyPath)
	if err != nil {
		return certFileContents{}, err
	}

	ret.certHash = blake2b.Sum256(ret.certBytes)
	ret.keyHash = blake2b.Sum256(ret.keyBytes)

	return ret, nil
}
