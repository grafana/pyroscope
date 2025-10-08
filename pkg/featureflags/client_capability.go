package featureflags

import (
	"context"
	"flag"
	"mime"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// Capability names - update parseClientCapabilities below when new capabilities added
	allowUtf8LabelNamesCapabilityName string = "allow-utf8-labelnames"

	// Config
	clientCapabilityPrefix = "client-capability."
	allowUtf8LabelNames    = clientCapabilityPrefix + allowUtf8LabelNamesCapabilityName
)

type ClientCapabilityConfig struct {
	AllowUtf8LabelNames bool `yaml:"allow_utf_8_label_names" category:"experimental"`
}

func (cfg *ClientCapabilityConfig) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(
		&cfg.AllowUtf8LabelNames,
		allowUtf8LabelNames,
		false,
		"Enable reading and writing utf-8 label names. To use this feature, API calls must "+
			"include `allow-utf8-labelnames=true` in the `Accept` header.",
	)
}

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

func ClientCapabilitiesGRPCMiddleware(cfg *ClientCapabilityConfig, logger log.Logger) grpc.UnaryServerInterceptor {
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
		clientCapabilities, err := parseClientCapabilities(httpHeader, cfg, logger)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}

		enhancedCtx := WithClientCapabilities(ctx, clientCapabilities)
		return handler(enhancedCtx, req)
	}
}

// ClientCapabilitiesHttpMiddleware creates middleware that extracts and parses the
// `Accept` header for capabilities the client supports
func ClientCapabilitiesHttpMiddleware(cfg *ClientCapabilityConfig, logger log.Logger) middleware.Interface {
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientCapabilities, err := parseClientCapabilities(r.Header, cfg, logger)
			if err != nil {
				http.Error(w, "Invalid header format: "+err.Error(), http.StatusBadRequest)
				return
			}

			ctx := WithClientCapabilities(r.Context(), clientCapabilities)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}

func parseClientCapabilities(header http.Header, cfg *ClientCapabilityConfig, logger log.Logger) (ClientCapabilities, error) {
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
								if !cfg.AllowUtf8LabelNames {
									level.Warn(logger).Log(
										"msg", "client requested capability that is not enabled on server",
										"capability", allowUtf8LabelNamesCapabilityName)
								} else {
									capabilities.AllowUtf8LabelNames = true
								}
							}
						default:
							level.Debug(logger).Log(
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
