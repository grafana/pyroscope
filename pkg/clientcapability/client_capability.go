package clientcapability

import (
	"context"
	"mime"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	// Capability names
	AllowUtf8LabelNamesCapabilityName CapabilityName = "allow-utf8-labelnames"
)

var capabilities = map[CapabilityName]bool{
	AllowUtf8LabelNamesCapabilityName: true,
}

type CapabilityName string
type CapabilityValue string

// Define a custom context key type to avoid collisions
type contextKey int

const (
	clientCapabilitiesKey contextKey = iota
	acceptHeader                     = "Accept"
)

type ClientCapabilities map[CapabilityName]CapabilityValue

func WithClientCapabilities(ctx context.Context, clientCapabilities ClientCapabilities) context.Context {
	return context.WithValue(ctx, clientCapabilitiesKey, clientCapabilities)
}

func GetClientCapabilities(ctx context.Context) (ClientCapabilities, bool) {
	value, ok := ctx.Value(clientCapabilitiesKey).(ClientCapabilities)
	return value, ok
}

func ClientCapabilitiesGRPCMiddleware() grpc.UnaryServerInterceptor {
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

		if len(clientCapabilities) == 0 {
			return handler(ctx, req)
		}

		enhancedCtx := WithClientCapabilities(ctx, clientCapabilities)
		return handler(enhancedCtx, req)
	}
}

// ClientCapabilitiesHttpMiddleware creates middleware that extracts and parses the
// `Accept` header for capabilities the client supports
func ClientCapabilitiesHttpMiddleware() middleware.Interface {
	return middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientCapabilities, err := parseClientCapabilities(r.Header)
			if err != nil {
				http.Error(w, "Invalid header format: "+err.Error(), http.StatusBadRequest)
				return
			} else if len(clientCapabilities) == 0 {
				// If no capabilities parsed, continue without setting context
				next.ServeHTTP(w, r)
				return
			}

			ctx := WithClientCapabilities(r.Context(), clientCapabilities)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
}

type ClientCapability struct {
	Name  CapabilityName
	Value string
}

func parseClientCapabilities(header http.Header) (ClientCapabilities, error) {
	acceptHeaderValues := header.Values(acceptHeader)

	var clientCapabilities = make(ClientCapabilities)
	for _, acceptHeaderValue := range acceptHeaderValues {
		if acceptHeaderValue != "" {
			accepts := strings.Split(acceptHeaderValue, ",")

			for _, accept := range accepts {
				if _, params, err := mime.ParseMediaType(accept); err != nil {
					return nil, err
				} else {
					for k, v := range params {
						if _, ok := capabilities[CapabilityName(k)]; ok {
							clientCapabilities[CapabilityName(k)] = CapabilityValue(v)
						}
					}
				}
			}
		}
	}
	return clientCapabilities, nil
}
