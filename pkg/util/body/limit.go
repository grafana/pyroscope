package body

import (
	"context"
	"net/http"

	"github.com/grafana/dskit/tenant"
)

type Limits interface {
	IngestionBodyLimitBytes(tenantID string) int64
}

func getMaxBodySize(ctx context.Context, limits Limits) int64 {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return 0
	}

	return limits.IngestionBodyLimitBytes(tenantID)
}

func NewSizeLimitHandler(limits Limits) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			maxBodySize := getMaxBodySize(r.Context(), limits)

			if maxBodySize > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
			}

			h.ServeHTTP(w, r)
		})
	}
}
