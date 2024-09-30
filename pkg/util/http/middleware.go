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
		return k6.LabelsFromBaggageHandler(h)
	})
}
