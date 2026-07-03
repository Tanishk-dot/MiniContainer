package observability

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType defines the type of metric being recorded
type MetricType int

const (
	MetricTypeCounter MetricType = iota
	MetricTypeGauge
	MetricTypeHistogram
	MetricTypeTimer
)

// Metric represents a recorded measurement
type Metric struct {
	Name      string
	Type      MetricType
	Value     float64
	Unit      string
	Labels    map[string]string
	Timestamp time.Time
}

// Counter is an incrementing metric
type Counter struct {
	name   string
	value  int64
	labels map[string]string
}

// NewCounter creates a new counter metric
func NewCounter(name string) *Counter {
	return &Counter{
		name:   name,
		value:  0,
		labels: make(map[string]string),
	}
}

// Inc increments the counter by 1
func (c *Counter) Inc() {
	c.Add(1)
}

// Add increments the counter by the given amount
func (c *Counter) Add(delta int64) {
	atomic.AddInt64(&c.value, delta)
}

// Value returns the current value
func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

// WithLabels returns a new counter with labels
func (c *Counter) WithLabels(labels map[string]string) *Counter {
	return &Counter{
		name:   c.name,
		value:  0,
		labels: labels,
	}
}

// Gauge is a metric that can go up or down
type Gauge struct {
	name   string
	value  int64
	labels map[string]string
}

// NewGauge creates a new gauge metric
func NewGauge(name string) *Gauge {
	return &Gauge{
		name:   name,
		value:  0,
		labels: make(map[string]string),
	}
}

// Set sets the gauge to a specific value
func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

// Inc increments the gauge
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

// Dec decrements the gauge
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

// Value returns the current value
func (g *Gauge) Value() int64 {
	return atomic.LoadInt64(&g.value)
}

// WithLabels returns a new gauge with labels
func (g *Gauge) WithLabels(labels map[string]string) *Gauge {
	return &Gauge{
		name:   g.name,
		value:  0,
		labels: labels,
	}
}

// Histogram tracks the distribution of values
type Histogram struct {
	name    string
	buckets []int64
	sum     int64
	count   int64
	labels  map[string]string
	mu      sync.Mutex
}

// NewHistogram creates a new histogram
func NewHistogram(name string) *Histogram {
	return &Histogram{
		name:    name,
		buckets: make([]int64, 10),
		sum:     0,
		count:   0,
		labels:  make(map[string]string),
	}
}

// Observe records an observation
func (h *Histogram) Observe(value int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += value

	// Determine bucket
	bucket := 0
	if value < 10 {
		bucket = 0
	} else if value < 50 {
		bucket = 1
	} else if value < 100 {
		bucket = 2
	} else if value < 500 {
		bucket = 3
	} else if value < 1000 {
		bucket = 4
	} else if value < 5000 {
		bucket = 5
	} else if value < 10000 {
		bucket = 6
	} else {
		bucket = 9
	}

	if bucket < len(h.buckets) {
		h.buckets[bucket]++
	}
}

// Count returns total observations
func (h *Histogram) Count() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// Sum returns sum of all values
func (h *Histogram) Sum() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sum
}

// Mean returns average value
func (h *Histogram) Mean() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return 0
	}
	return float64(h.sum) / float64(h.count)
}

// Timer tracks elapsed time for operations
type Timer struct {
	name      string
	start     time.Time
	histogram *Histogram
	labels    map[string]string
}

// NewTimer creates a new timer
func NewTimer(name string) *Timer {
	return &Timer{
		name:      name,
		start:     time.Now(),
		histogram: NewHistogram(name + "_duration_ms"),
		labels:    make(map[string]string),
	}
}

// Stop ends the timer and records the duration
func (t *Timer) Stop() int64 {
	duration := time.Since(t.start).Milliseconds()
	t.histogram.Observe(duration)
	return duration
}

// Duration returns elapsed time since start
func (t *Timer) Duration() time.Duration {
	return time.Since(t.start)
}

// MetricsRegistry manages all metrics
type MetricsRegistry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
	timers   map[string]*Timer
	metrics  []Metric
}

// NewMetricsRegistry creates a new metrics registry
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*Gauge),
		timers:   make(map[string]*Timer),
		metrics:  make([]Metric, 0),
	}
}

// RegisterCounter registers a new counter
func (mr *MetricsRegistry) RegisterCounter(name string) *Counter {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if c, exists := mr.counters[name]; exists {
		return c
	}

	c := NewCounter(name)
	mr.counters[name] = c
	return c
}

// RegisterGauge registers a new gauge
func (mr *MetricsRegistry) RegisterGauge(name string) *Gauge {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if g, exists := mr.gauges[name]; exists {
		return g
	}

	g := NewGauge(name)
	mr.gauges[name] = g
	return g
}

// RegisterTimer registers a new timer
func (mr *MetricsRegistry) RegisterTimer(name string) *Timer {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if t, exists := mr.timers[name]; exists {
		return t
	}

	t := NewTimer(name)
	mr.timers[name] = t
	return t
}

// GetMetrics returns all recorded metrics
func (mr *MetricsRegistry) GetMetrics() []Metric {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	metrics := make([]Metric, 0)

	// Collect counter metrics
	for name, counter := range mr.counters {
		metrics = append(metrics, Metric{
			Name:      name,
			Type:      MetricTypeCounter,
			Value:     float64(counter.Value()),
			Unit:      "count",
			Labels:    counter.labels,
			Timestamp: time.Now(),
		})
	}

	// Collect gauge metrics
	for name, gauge := range mr.gauges {
		metrics = append(metrics, Metric{
			Name:      name,
			Type:      MetricTypeGauge,
			Value:     float64(gauge.Value()),
			Unit:      "value",
			Labels:    gauge.labels,
			Timestamp: time.Now(),
		})
	}

	// Collect timer metrics
	for name, timer := range mr.timers {
		metrics = append(metrics, Metric{
			Name:      name + "_count",
			Type:      MetricTypeCounter,
			Value:     float64(timer.histogram.Count()),
			Unit:      "count",
			Labels:    timer.labels,
			Timestamp: time.Now(),
		})

		metrics = append(metrics, Metric{
			Name:      name + "_mean_ms",
			Type:      MetricTypeGauge,
			Value:     timer.histogram.Mean(),
			Unit:      "ms",
			Labels:    timer.labels,
			Timestamp: time.Now(),
		})
	}

	return append(metrics, mr.metrics...)
}

// RecordMetric manually records a metric
func (mr *MetricsRegistry) RecordMetric(m Metric) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	m.Timestamp = time.Now()
	mr.metrics = append(mr.metrics, m)
}

// SystemMetrics provides system-level metrics
type SystemMetrics struct {
	MemoryAlloc        uint64
	MemoryTotalAlloc   uint64
	MemorySys          uint64
	MemoryNumGC        uint32
	GoroutineCount     int
	CPUUsagePercent    float64
	UptimeSeconds      int64
	StartTime          time.Time
}

// CollectSystemMetrics gathers current system metrics
func CollectSystemMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemMetrics{
		MemoryAlloc:      m.Alloc,
		MemoryTotalAlloc: m.TotalAlloc,
		MemorySys:        m.Sys,
		MemoryNumGC:      m.NumGC,
		GoroutineCount:   runtime.NumGoroutine(),
		UptimeSeconds:    int64(time.Since(globalStartTime).Seconds()),
		StartTime:        globalStartTime,
	}
}

var globalStartTime = time.Now()

// ContextWithMetrics adds metrics registry to context
func ContextWithMetrics(ctx context.Context, registry *MetricsRegistry) context.Context {
	return context.WithValue(ctx, metricsKey{}, registry)
}

// MetricsFromContext extracts metrics registry from context
func MetricsFromContext(ctx context.Context) *MetricsRegistry {
	mr, ok := ctx.Value(metricsKey{}).(*MetricsRegistry)
	if !ok {
		return NewMetricsRegistry()
	}
	return mr
}

type metricsKey struct{}

// GlobalMetrics is the global metrics registry
var GlobalMetrics = NewMetricsRegistry()

// Common metric names
const (
	// HTTP API metrics
	HTTPRequestTotal    = "http_requests_total"
	HTTPRequestDuration = "http_request_duration_ms"
	HTTPErrorsTotal     = "http_errors_total"

	// Registry metrics
	BlobPushTotal       = "registry_blob_push_total"
	BlobPullTotal       = "registry_blob_pull_total"
	BlobDeleteTotal     = "registry_blob_delete_total"
	ManifestPushTotal   = "registry_manifest_push_total"
	ManifestPullTotal   = "registry_manifest_pull_total"
	BlobUploadDuration  = "registry_blob_upload_duration_ms"

	// Scheduler metrics
	DeploymentTotal     = "scheduler_deployments_total"
	ContainerRunning    = "scheduler_containers_running"
	ContainerStarts     = "scheduler_container_starts_total"
	ContainerStops      = "scheduler_container_stops_total"
	DeploymentDuration  = "scheduler_deployment_duration_ms"

	// Storage metrics
	StorageSize        = "storage_size_bytes"
	StorageObjectCount = "storage_object_count"

	// System metrics
	SystemMemoryBytes   = "system_memory_bytes"
	SystemGoroutines    = "system_goroutines"
	SystemUptimeSeconds = "system_uptime_seconds"
)
