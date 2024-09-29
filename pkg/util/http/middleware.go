package http

import (
	"net/http"

	"github.com/grafana/dskit/middleware"
	"github.com/grafana/pyroscope-go/x/k6"
	"go.opentelemetry.io/otel/baggage"
)

// K6Middleware creates a middleware that extracts k6 load test labels from the
// request baggage and adds them as dynamic profiling labels.
func K6Middleware() middleware.Interface {
	return middleware.Func(func(h http.Handler) http.Handler {
		next := k6.LabelsFromBaggageHandler(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = setBaggageRequestContext(r)
			next.ServeHTTP(w, r)
		})
	})
}

// SetBaggageContext sets the Baggage in the request context if it exists as a
// header.
//
// TODO(bryan) Move this into the pyroscope-go/x/k6 package.
func setBaggageRequestContext(r *http.Request) *http.Request {
	b, err := baggage.Parse(r.Header.Get("Baggage"))
	if err != nil {
		return r
	}

	ctx := baggage.ContextWithBaggage(r.Context(), b)
	return r.WithContext(ctx)
}
