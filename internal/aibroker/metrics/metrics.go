package metrics

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aibroker_requests_total",
			Help: "Total number of requests processed by the AI broker",
		},
		[]string{"provider", "code"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aibroker_request_duration_seconds",
			Help:    "Duration of requests processed by the AI broker",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider"},
	)

	upstreamErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aibroker_upstream_errors_total",
			Help: "Total number of upstream errors from AI providers",
		},
		[]string{"provider"},
	)
)

type Logger struct {
	*slog.Logger
}

func NewLogger(level string) *Logger {
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slogLevel,
	}))

	return &Logger{Logger: logger}
}

func (l *Logger) LogRequest(provider, method, path string, code int, latency time.Duration, err error) {
	// Strip authorization headers and other sensitive data for logging
	logLevel := slog.LevelInfo
	if code >= 400 {
		logLevel = slog.LevelWarn
	}
	if code >= 500 || err != nil {
		logLevel = slog.LevelError
	}

	args := []any{
		"provider", provider,
		"method", method,
		"path", sanitizePath(path),
		"status_code", code,
		"latency_ms", latency.Milliseconds(),
	}

	if err != nil {
		args = append(args, "error", err.Error())
	}

	l.Log(nil, logLevel, "Request processed", args...)
}

func RecordRequest(provider string, code int, latency time.Duration) {
	codeStr := strconv.Itoa(code)
	requestsTotal.WithLabelValues(provider, codeStr).Inc()
	requestDuration.WithLabelValues(provider).Observe(latency.Seconds())

	if code == 429 || code >= 500 {
		upstreamErrors.WithLabelValues(provider).Inc()
	}
}

// sanitizePath removes potential sensitive information from paths while keeping them useful for logging
func sanitizePath(path string) string {
	// Keep only the first few path segments to avoid logging sensitive IDs or tokens
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return strings.Join(parts[:3], "/") + "/..."
	}
	return path
}

type ResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *ResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseWriter) StatusCode() int {
	return rw.statusCode
}