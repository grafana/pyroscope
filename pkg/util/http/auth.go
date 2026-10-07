package http

import (
	"net/http"

	"github.com/grafana/dskit/middleware"
	"github.com/grafana/dskit/user"

	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

// AuthenticateUser propagates the user ID from HTTP headers back to the request's context.
// If on is false, it will inject the default tenant ID.
func AuthenticateUser(on bool) middleware.Interface {
	// TODO: @petethepig This logic is copied in otlp.*ingestHandler.Export. We should unify
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !on {
				next.ServeHTTP(w, r.WithContext(user.InjectOrgID(r.Context(), tenant.DefaultTenantID)))
				return
			}
			_, ctx, err := user.ExtractOrgIDFromHTTPRequest(r)
			if err != nil {
				ErrorWithStatus(w, err, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}
