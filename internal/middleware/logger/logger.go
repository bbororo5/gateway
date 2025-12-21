package logger

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jeonseon-ung/gateway/pkg/utils"
	"github.com/google/uuid"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += int64(n)
	return n, err
}

func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		requestID := processRequest(r)

		rr := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rr, r)

		logResponse(r, rr, requestID, start)
	})
}

func processRequest(r *http.Request) string {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.NewString()
		r.Header.Set("X-Request-ID", requestID)
	}
	return requestID
}

func logResponse(r *http.Request, rr *responseRecorder, requestID string, start time.Time) {
	duration := time.Since(start)

	clientIP := utils.GetClientIP(r)

	attrs := []any{
		slog.String("request_id", requestID),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("remote_ip", clientIP), // 수정됨
		slog.Int("status", rr.statusCode),
		slog.Int64("size", rr.size),
		slog.Int64("duration_ms", duration.Milliseconds()),
		slog.String("user_agent", r.UserAgent()),
	}

	if rr.statusCode >= 500 {
		slog.Error("HTTP Server Error", attrs...)
	} else if rr.statusCode >= 400 {
		slog.Warn("HTTP Client Error", attrs...)
	} else {
		slog.Info("HTTP Access Log", attrs...)
	}
}
