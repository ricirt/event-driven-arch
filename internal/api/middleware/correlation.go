package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const correlationIDKey contextKey = "correlation_id"

// CorrelationID reads the X-Correlation-ID header from the incoming request.
// If absent, a new UUID is generated. The value is stored on the request
// context and echoed back in the response header so callers can trace
// their request through logs.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = uuid.New().String()
		}
		ctx := context.WithValue(r.Context(), correlationIDKey, id)
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetCorrelationID retrieves the correlation ID stored by the middleware.
// Returns an empty string if the middleware was not applied.
func GetCorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(correlationIDKey).(string)
	return v
}
