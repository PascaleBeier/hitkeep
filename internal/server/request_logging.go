package server

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"hitkeep/internal/server/shared"
)

const maxRequestIDLength = 128

func (s *Server) requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFromRequest(r)
		logger := s.logger.With("request_id", requestID)
		requestLogger := logger.With(slog.Group(
			"http",
			"method", r.Method,
			"path", safeRequestPath(r.URL.Path),
		))
		w.Header().Set("X-Request-Id", requestID)

		started := time.Now()
		response := &statusResponseWriter{ResponseWriter: w}
		request := r.WithContext(shared.WithLogger(r.Context(), requestLogger))
		next.ServeHTTP(response, request)

		status := response.statusCode()
		level := slog.LevelDebug
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		logger.Log(request.Context(), level, "HTTP request completed", slog.Group(
			"http",
			"method", r.Method,
			"path", safeRequestPath(r.URL.Path),
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
		))
	})
}

func requestIDFromRequest(r *http.Request) string {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if requestID != "" && len(requestID) <= maxRequestIDLength && strings.IndexFunc(requestID, unicode.IsControl) == -1 {
		return requestID
	}
	return uuid.NewString()
}

func safeRequestPath(rawPath string) string {
	leadingSlash := strings.HasPrefix(rawPath, "/")
	segments := strings.Split(strings.TrimPrefix(rawPath, "/"), "/")
	for index := 1; index < len(segments); index++ {
		switch segments[index-1] {
		case "share", "qr-share", "q", "on-demand-tls", "report-recipient-confirmations", "unsubscribe":
			segments[index] = "[redacted]"
		}
	}
	path := strings.Join(segments, "/")
	if leadingSlash {
		return "/" + path
	}
	return path
}

type statusResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	wroteFinal  bool
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.wroteFinal {
		return
	}
	w.wroteHeader = true
	w.wroteFinal = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(r)
	}
	return io.Copy(w.ResponseWriter, r)
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusResponseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
