package httputil

import (
	"log/slog"
	"net/http"
	"time"
)

func LoggingMiddleware(onlyErrors bool) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			didAbort := true
			startTime := time.Now()

			logLevel := slog.LevelInfo
			if onlyErrors {
				logLevel = slog.LevelDebug
			}

			header := r.Header

			slog.LogAttrs(ctx, logLevel, "request",
				slog.String("method", r.Method),
				slog.String("route", r.Pattern),
				slog.String("path", r.RequestURI),
				slog.String("content-length", header.Get("content-length")),
				slog.String("content-type", header.Get("content-type")),
			)

			recorder := &recordingResponseWriter{w: w}

			defer func() {
				if recorder.sc == 0 && !didAbort {
					// not the greatest but it's mimics what the stdlib will
					// actually send back.
					recorder.sc = 200
				}
				if recorder.sc >= 400 && recorder.sc < 500 {
					logLevel = slog.LevelWarn
				} else if recorder.sc >= 500 {
					logLevel = slog.LevelError
				}
				if didAbort && logLevel < slog.LevelWarn {
					logLevel = slog.LevelWarn
				}

				slog.LogAttrs(ctx, logLevel, "response",
					slog.String("method", r.Method),
					slog.String("route", r.Pattern),
					slog.String("path", r.RequestURI),
					slog.Int("status", recorder.sc),
					slog.Bool("didAbort", didAbort),
					slog.Duration("duration", time.Since(startTime)),
					slog.Int64("wrote", recorder.wrote),
				)
			}()

			h.ServeHTTP(recorder, r)
			didAbort = false
		})
	}
}

type recordingResponseWriter struct {
	w           http.ResponseWriter
	wrote       int64
	wroteHeader bool
	sc          int
}

// Header implements http.ResponseWriter.
func (r *recordingResponseWriter) Header() http.Header {
	return r.w.Header()
}

// Write implements http.ResponseWriter.
func (r *recordingResponseWriter) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
		r.sc = http.StatusOK
	}
	wrote, err := r.w.Write(data)
	r.wrote += int64(wrote)
	return wrote, err
}

// WriteHeader implements http.ResponseWriter.
func (r *recordingResponseWriter) WriteHeader(statusCode int) {
	if !r.wroteHeader {
		r.wroteHeader = true
		r.sc = statusCode
	}
	r.w.WriteHeader(statusCode)
}

func (r *recordingResponseWriter) Unwrap() http.ResponseWriter {
	return r.w
}

var _ http.ResponseWriter = (*recordingResponseWriter)(nil)
