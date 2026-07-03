package observability

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPMiddleware provides observability for HTTP handlers
type HTTPMiddleware struct {
	metrics *MetricsRegistry
	logger  *Logger
}

// NewHTTPMiddleware creates a new HTTP middleware
func NewHTTPMiddleware(logger *Logger, metrics *MetricsRegistry) *HTTPMiddleware {
	return &HTTPMiddleware{
		metrics: metrics,
		logger:  logger,
	}
}

// Middleware wraps an HTTP handler with observability
func (hm *HTTPMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create trace context
		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = generateID()
		}

		tc := NewTraceContext().WithTraceID(traceID)
		r = r.WithContext(ContextWithTrace(r.Context(), tc))

		// Record request metrics
		operation := NewOperation(fmt.Sprintf("%s %s", r.Method, r.URL.Path), tc)
		operation.Start()

		// Create response writer wrapper to capture status and bytes
		wrapped := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Handle request
		start := time.Now()
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)

		// Record operation completion
		operation.SetAttribute("status_code", wrapped.statusCode)
		operation.SetAttribute("duration_ms", duration.Milliseconds())
		operation.SetAttribute("bytes_written", wrapped.bytesWritten)

		if wrapped.statusCode >= 400 {
			operation.Error(fmt.Errorf("HTTP %d", wrapped.statusCode))
		} else {
			operation.End()
		}

		// Log request
		hm.logger.Info(fmt.Sprintf("%s %s %d", r.Method, r.URL.Path, wrapped.statusCode),
			map[string]interface{}{
				"trace_id":      tc.TraceID,
				"span_id":       tc.SpanID,
				"method":        r.Method,
				"path":          r.URL.Path,
				"status_code":   wrapped.statusCode,
				"duration_ms":   duration.Milliseconds(),
				"bytes_written": wrapped.bytesWritten,
				"remote_addr":   r.RemoteAddr,
			})

		// Record metrics
		if hm.metrics != nil {
			counter := hm.metrics.RegisterCounter(HTTPRequestTotal)
			counter.Inc()

			if wrapped.statusCode >= 400 {
				errCounter := hm.metrics.RegisterCounter(HTTPErrorsTotal)
				errCounter.Inc()
			}

			// Record latency histogram
			histogram := hm.metrics.RegisterTimer(HTTPRequestDuration)
			histogram.histogram.Observe(duration.Milliseconds())
		}

		// Set trace headers in response
		w.Header().Set("X-Trace-ID", tc.TraceID)
		w.Header().Set("X-Span-ID", tc.SpanID)
	})
}

// responseWriterWrapper wraps http.ResponseWriter to capture status and bytes
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

// WriteHeader captures the status code
func (r *responseWriterWrapper) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write captures bytes written
func (r *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += n
	return n, err
}

// HealthCheckHandler provides a health check endpoint
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// Collect system metrics
	sysMet := CollectSystemMetrics()

	response := map[string]interface{}{
		"status":     "healthy",
		"timestamp":  time.Now(),
		"uptime_sec": sysMet.UptimeSeconds,
		"goroutines": sysMet.GoroutineCount,
		"memory": map[string]interface{}{
			"alloc_bytes":       sysMet.MemoryAlloc,
			"total_alloc_bytes": sysMet.MemoryTotalAlloc,
			"sys_bytes":         sysMet.MemorySys,
			"num_gc":            sysMet.MemoryNumGC,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, toJSON(response))
}

// MetricsHandler provides metrics endpoint
func MetricsHandler(mr *MetricsRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics := mr.GetMetrics()

		// Convert to JSON-friendly format
		output := make([]map[string]interface{}, 0, len(metrics))
		for _, m := range metrics {
			output = append(output, map[string]interface{}{
				"name":      m.Name,
				"type":      m.Type,
				"value":     m.Value,
				"unit":      m.Unit,
				"labels":    m.Labels,
				"timestamp": m.Timestamp,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, toJSON(output))
	}
}

// DebugHandler provides debug information
func DebugHandler(w http.ResponseWriter, r *http.Request) {
	sysMet := CollectSystemMetrics()

	response := map[string]interface{}{
		"system": map[string]interface{}{
			"uptime_sec":           sysMet.UptimeSeconds,
			"goroutines":           sysMet.GoroutineCount,
			"memory_alloc_mb":      float64(sysMet.MemoryAlloc) / 1024 / 1024,
			"memory_total_alloc_mb": float64(sysMet.MemoryTotalAlloc) / 1024 / 1024,
			"memory_sys_mb":        float64(sysMet.MemorySys) / 1024 / 1024,
			"gc_runs":              sysMet.MemoryNumGC,
		},
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, toJSON(response))
}

// toJSON converts a value to JSON string (simple implementation)
func toJSON(v interface{}) string {
	// Simple JSON stringification
	switch val := v.(type) {
	case map[string]interface{}:
		parts := make([]string, 0)
		for k, v := range val {
			parts = append(parts, fmt.Sprintf(`"%s":%s`, k, toJSON(v)))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []map[string]interface{}:
		parts := make([]string, 0)
		for _, m := range val {
			parts = append(parts, toJSON(m))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case string:
		return fmt.Sprintf(`"%s"`, val)
	case float64, int, int64:
		return fmt.Sprintf("%v", val)
	case time.Time:
		return fmt.Sprintf(`"%s"`, val.Format(time.RFC3339))
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ErrorHandler logs errors with trace context
func ErrorHandler(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	tc := TraceFromContext(r.Context())

	// Log error
	logger := GlobalLogger.Get("http")
	logger.Error(message, map[string]interface{}{
		"trace_id":    tc.TraceID,
		"span_id":     tc.SpanID,
		"status_code": statusCode,
		"path":        r.URL.Path,
	})

	// Record metric
	if counter := GlobalMetrics.RegisterCounter(HTTPErrorsTotal); counter != nil {
		counter.Inc()
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Trace-ID", tc.TraceID)
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error":"%s","trace_id":"%s"}`, message, tc.TraceID)
}
