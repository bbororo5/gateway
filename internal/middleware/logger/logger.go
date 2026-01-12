package logger

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jeonseon-ung/gateway/internal/httpstats"
	"github.com/jeonseon-ung/gateway/pkg/utils"
)

func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		r = r.Clone(r.Context())
		requestID := processRequest(r)
		w.Header().Set("X-Request-ID", requestID)
		rr := &httpstats.Response{ResponseWriter: w}

		next.ServeHTTP(rr, r)

		logResponse(r, rr, requestID, start)
	})
}

func processRequest(r *http.Request) string {
	requestID := r.Header.Get("X-Request-ID")
	if !validRequestID(requestID) {
		requestID = uuid.NewString()
		r.Header.Set("X-Request-ID", requestID)
	}
	return requestID
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func logResponse(r *http.Request, rr *httpstats.Response, requestID string, start time.Time) {
	duration := time.Since(start)

	clientIP := utils.GetClientIP(r)

	attrs := []any{
		slog.String("request_id", requestID),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("remote_ip", clientIP),
		slog.Int("status", rr.StatusCode()),
		slog.Int64("size", rr.Bytes),
		slog.Int64("duration_ms", duration.Milliseconds()),
		slog.String("user_agent", r.UserAgent()),
	}

	if rr.StatusCode() >= 500 {
		slog.Error("HTTP Server Error", attrs...)
	} else if rr.StatusCode() >= 400 {
		slog.Warn("HTTP Client Error", attrs...)
	} else {
		slog.Info("HTTP Access Log", attrs...)
	}
}
