package tenant

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/dskit/user"
)

// DefaultTenantID is the default tenant ID used when the interceptor is disabled.
const DefaultTenantID = "anonymous"

// NewAuthInterceptor create a new tenant authentication interceptor for the server and client.
//
// For the server:
//
// If enabled, the interceptor will check the tenant ID in the request header is present and inject it into the context.
// When the interceptor is disabled, it will inject the default tenant ID into the context.
//
// For the client :
//
// The interceptor will inject the tenant ID from the context into the request header no matter if the interceptor is enabled or not.
func NewAuthInterceptor(enabled bool) connect.Interceptor {
	return &authInterceptor{enabled: enabled}
}

type authInterceptor struct {
	enabled bool
}

func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// client side we extract the tenantID from the context and inject it into the request header
		if req.Spec().IsClient {
			tenantID, _ := ExtractTenantIDFromContext(ctx)
			if tenantID != "" {
				req.Header().Set("X-Scope-OrgID", tenantID)
			}
			return next(ctx, req)
		}
		// Server side if the interceptor is enabled, we extract the tenantID from the request header and inject it into the context
		// If the interceptor is disabled, we inject the default tenant ID into the context.
		if !i.enabled {
			return next(InjectTenantID(ctx, DefaultTenantID), req)
		}
		_, ctx, _ = ExtractTenantIDFromHeaders(ctx, req.Header())

		resp, err := next(ctx, req)
		if err != nil && errors.Is(err, ErrNoTenantID) {
			return resp, connect.NewError(connect.CodeUnauthenticated, err)
		}
		return resp, err
	}
}

func (i *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, s connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, s)
		tenantID, _ := ExtractTenantIDFromContext(ctx)
		if tenantID != "" {
			conn.RequestHeader().Set("X-Scope-OrgID", tenantID)
		}
		return conn
	}
}

func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !i.enabled {
			return next(InjectTenantID(ctx, DefaultTenantID), conn)
		}
		_, ctx, _ = ExtractTenantIDFromHeaders(ctx, conn.RequestHeader())
		if err := next(ctx, conn); err != nil {
			if errors.Is(err, ErrNoTenantID) {
				return connect.NewError(connect.CodeUnauthenticated, err)
			}
			return err
		}
		return nil
	}
}

var defaultResolver tenant.Resolver = tenant.NewMultiResolver()

// ExtractTenantIDFromHeaders extracts a single TenantID from http headers.
func ExtractTenantIDFromHeaders(ctx context.Context, headers http.Header) (string, context.Context, error) {
	orgID := headers.Get(user.OrgIDHeaderName)
	if orgID == "" {
		return "", ctx, ErrNoTenantID
	}
	ctx = InjectTenantID(ctx, orgID)

	tenantID, err := defaultResolver.TenantID(ctx)
	if err != nil {
		return "", nil, err
	}

	return tenantID, ctx, nil
}

// ExtractTenantIDFromContext extracts a single TenantID from the context.
func ExtractTenantIDFromContext(ctx context.Context) (string, error) {
	tenantID, err := defaultResolver.TenantID(ctx)
	if err != nil {
		return "", err
	}

	return tenantID, nil
}
