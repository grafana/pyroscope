package featureflags

import (
	"context"
	"mime"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/middleware"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// Capability names - update parseClientCapabilities and trackEnabledCapabilities
	// below when new capabilities added
	allowUtf8LabelNamesCapabilityName string = "allow-utf8-labelnames"
)

var (
	usage = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "client_capability_enabled_total",
			Help: "Total number of requests with client capabilities enabled",
		},
		[]string{"tenant", "capability_name"},
	)
)

// Define a custom context key type to avoid collisions
type contextKey struct{}

type ClientCapabilities struct {
	AllowUtf8LabelNames bool
}

func WithClientCapabilities(ctx context.Context, clientCapabilities ClientCapabilities) context.Context {
	return context.WithValue(ctx, contextKey{}, clientCapabilities)
}

func GetClientCapabilities(ctx context.Context) (ClientCapabilities, bool) {
	value, ok := ctx.Value(contextKey{}).(ClientCapabilities)
	return value, ok
}

type ClientCapabilityMiddleware struct {
	usage *prometheus.CounterVec
}

func NewClientCapabilityMiddleware(reg prometheus.Registerer) *ClientCapabilityMiddleware {
	usage = util.RegisterOrGet(reg, usage)

	return &ClientCapabilityMiddleware{
		usage: usage,
	}
}

// trackEnabledCapabilities records metrics for each enabled capability
func (m *ClientCapabilityMiddleware) trackEnabledCapabilities(ctx context.Context, capabilities ClientCapabilities) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		// Fall back if tenant cannot be extracted
		tenantID = tenant.DefaultTenantID
	}

	// Track each enabled capability
	if capabilities.AllowUtf8LabelNames {
		m.usage.WithLabelValues(tenantID, allowUtf8LabelNamesCapabilityName).Inc()
	}
}

// CreateGRPC creates gRPC middleware that extracts and parses the
// `Accept` header for capabilities the client supports
func (m *ClientCapabilityMiddleware) CreateGRPC() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract metadata from context
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return handler(ctx, req)
		}

		// Convert metadata to http.Header for reuse of existing parsing logic
		httpHeader := make(http.Header)
		for key, values := range md {
			// gRPC metadata keys are lowercase, HTTP headers are case-insensitive
			httpHeader[http.CanonicalHeaderKey(key)] = values
		}

		// Reuse existing HTTP header parsing
		clientCapabilities, err := parseClientCapabilities(httpHeader)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}

		enhancedCtx := WithClientCapabilities(ctx, clientCapabilities)

		// Track enabled capabilities for metrics
		m.trackEnabledCapabilities(enhancedCtx, clientCapabilities)

		return handler(enhancedCtx, req)
	}
}

// CreateHttp creates HTTP middleware that extracts and parses the
// `Accept` header for capabilities the client supports
func (m *ClientCapabilityMiddleware) CreateHttp() middleware.Interface {
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientCapabilities, err := parseClientCapabilities(r.Header)
			if err != nil {
				http.Error(w, "Invalid header format: "+err.Error(), http.StatusBadRequest)
				return
			}

			ctx := WithClientCapabilities(r.Context(), clientCapabilities)

			// Track enabled capabilities for metrics
			m.trackEnabledCapabilities(ctx, clientCapabilities)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}

func parseClientCapabilities(header http.Header) (ClientCapabilities, error) {
	acceptHeaderValues := header.Values("Accept")

	var capabilities ClientCapabilities

	for _, acceptHeaderValue := range acceptHeaderValues {
		if acceptHeaderValue != "" {
			accepts := strings.Split(acceptHeaderValue, ",")

			for _, accept := range accepts {
				if _, params, err := mime.ParseMediaType(accept); err != nil {
					return capabilities, err
				} else {
					for k, v := range params {
						switch k {
						case allowUtf8LabelNamesCapabilityName:
							if v == "true" {
								capabilities.AllowUtf8LabelNames = true
							}
						default:
							level.Debug(util.Logger).Log(
								"msg", "unknown capability parsed from Accept header",
								"acceptHeaderKey", k,
								"acceptHeaderValue", v)
						}
					}
				}
			}
		}
	}
	return capabilities, nil
}
