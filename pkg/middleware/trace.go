package middleware

import (
	"context"
	
	"github.com/google/uuid"
)

type contextKey string

const (
	TraceIDKey contextKey = "trace_id"
)

// GenerateTraceID generates a new trace ID
func GenerateTraceID() string {
	return uuid.New().String()
}

// WithTraceID adds a trace ID to the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID gets the trace ID from the context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}