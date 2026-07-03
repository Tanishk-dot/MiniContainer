package observability

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
)

// TraceContext holds tracing information for a request
type TraceContext struct {
	TraceID   string
	SpanID    string
	RequestID string
	UserID    string
	Metadata  map[string]string
}

// NewTraceContext creates a new trace context with generated IDs
func NewTraceContext() *TraceContext {
	return &TraceContext{
		TraceID:  generateID(),
		SpanID:   generateID(),
		RequestID: generateID(),
		Metadata: make(map[string]string),
	}
}

// WithTraceID sets the trace ID
func (tc *TraceContext) WithTraceID(id string) *TraceContext {
	tc.TraceID = id
	return tc
}

// WithSpanID sets the span ID
func (tc *TraceContext) WithSpanID(id string) *TraceContext {
	tc.SpanID = id
	return tc
}

// WithUserID sets the user ID
func (tc *TraceContext) WithUserID(id string) *TraceContext {
	tc.UserID = id
	return tc
}

// WithMetadata adds metadata
func (tc *TraceContext) WithMetadata(key, value string) *TraceContext {
	tc.Metadata[key] = value
	return tc
}

// NewSpan creates a child span with the same trace ID
func (tc *TraceContext) NewSpan() *TraceContext {
	return &TraceContext{
		TraceID:   tc.TraceID,
		SpanID:    generateID(),
		RequestID: tc.RequestID,
		UserID:    tc.UserID,
		Metadata:  tc.Metadata,
	}
}

// generateID generates a random ID for tracing
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%016x", b)
}

// ContextWithTrace adds trace context to context
func ContextWithTrace(ctx context.Context, tc *TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}

// TraceFromContext extracts trace context from context
func TraceFromContext(ctx context.Context) *TraceContext {
	tc, ok := ctx.Value(traceContextKey{}).(*TraceContext)
	if !ok {
		return NewTraceContext()
	}
	return tc
}

type traceContextKey struct{}

// ParseTraceHeader parses W3C Trace Context header format
func ParseTraceHeader(header string) *TraceContext {
	// Format: traceparent: version-traceID-parentID-traceFlags
	parts := strings.Split(header, "-")
	tc := NewTraceContext()

	if len(parts) >= 2 {
		tc.TraceID = parts[1]
	}
	if len(parts) >= 3 {
		tc.SpanID = parts[2]
	}

	return tc
}

// TraceHeaderString returns the W3C Trace Context header format
func (tc *TraceContext) TraceHeaderString() string {
	return fmt.Sprintf("00-%s-%s-01", tc.TraceID, tc.SpanID)
}

// Operation tracks a single operation within a span
type Operation struct {
	name       string
	traceCtx   *TraceContext
	start      int64 // nanoseconds
	end        int64
	status     string
	error      error
	attributes map[string]interface{}
}

// NewOperation creates a new operation
func NewOperation(name string, tc *TraceContext) *Operation {
	return &Operation{
		name:       name,
		traceCtx:   tc.NewSpan(),
		start:      0,
		end:        0,
		status:     "pending",
		attributes: make(map[string]interface{}),
	}
}

// Start marks the operation as started
func (o *Operation) Start() {
	o.status = "started"
}

// End marks the operation as ended with success
func (o *Operation) End() {
	o.status = "success"
}

// Error marks the operation as ended with error
func (o *Operation) Error(err error) {
	o.status = "error"
	o.error = err
}

// SetAttribute sets an operation attribute
func (o *Operation) SetAttribute(key string, value interface{}) {
	o.attributes[key] = value
}

// ToFields converts operation to log fields
func (o *Operation) ToFields() map[string]interface{} {
	fields := map[string]interface{}{
		"operation": o.name,
		"trace_id":  o.traceCtx.TraceID,
		"span_id":   o.traceCtx.SpanID,
		"status":    o.status,
	}

	if o.error != nil {
		fields["error"] = o.error.Error()
	}

	for k, v := range o.attributes {
		fields[k] = v
	}

	return fields
}
