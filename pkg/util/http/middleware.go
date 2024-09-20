package http

import (
	"net/http"

	"github.com/grafana/dskit/middleware"
	"github.com/grafana/pyroscope-go/x/k6"
)

// K6Middleware creates a middleware that extracts k6 load test labels from the
// request baggage and adds them as dynamic profiling labels.
func K6Middleware() middleware.Interface {
	return middleware.Func(func(h http.Handler) http.Handler {
		next := k6.LabelsFromBaggageHandler(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	})
}
