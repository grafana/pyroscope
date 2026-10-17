package featureflags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const utf8LabelNamesSpanAttr = attribute.Key("client_capability.allow_utf8_labelnames")

func TestClientCapabilitiesHttpMiddleware_SpanAttributes(t *testing.T) {
	tests := []struct {
		name       string
		accept     string
		wantStatus int
		wantAttrs  []attribute.KeyValue
	}{
		{
			name:       "capability enabled",
			accept:     "*/*; allow-utf8-labelnames=true",
			wantStatus: http.StatusOK,
			wantAttrs:  []attribute.KeyValue{utf8LabelNamesSpanAttr.Bool(true)},
		},
		{
			name:       "capability not sent",
			accept:     "application/json",
			wantStatus: http.StatusOK,
			wantAttrs:  []attribute.KeyValue{utf8LabelNamesSpanAttr.Bool(false)},
		},
		{
			name:       "invalid Accept header",
			accept:     "invalid/mime/type/format",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := ClientCapabilitiesHttpMiddleware().Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			rec := httptest.NewRecorder()

			attrs := recordSpanAttributes(t, func(ctx context.Context) {
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
				req.Header.Set("Accept", tt.accept)
				h.ServeHTTP(rec, req)
			})

			require.Equal(t, tt.wantStatus, rec.Code)
			require.ElementsMatch(t, tt.wantAttrs, attrs)
		})
	}
}

func TestClientCapabilitiesGRPCMiddleware_SpanAttributes(t *testing.T) {
	tests := []struct {
		name      string
		accept    string
		wantCode  connect.Code // zero means no error
		wantAttrs []attribute.KeyValue
	}{
		{
			name:      "capability enabled",
			accept:    "*/*; allow-utf8-labelnames=true",
			wantAttrs: []attribute.KeyValue{utf8LabelNamesSpanAttr.Bool(true)},
		},
		{
			name:      "capability not sent",
			accept:    "application/json",
			wantAttrs: []attribute.KeyValue{utf8LabelNamesSpanAttr.Bool(false)},
		},
		{
			name:     "invalid Accept header",
			accept:   "invalid/mime/type/format",
			wantCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := func(context.Context, any) (any, error) { return nil, nil }
			var err error

			attrs := recordSpanAttributes(t, func(ctx context.Context) {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("accept", tt.accept))
				_, err = ClientCapabilitiesGRPCMiddleware()(ctx, nil, &grpc.UnaryServerInfo{}, handler)
			})

			if tt.wantCode != 0 {
				require.Equal(t, tt.wantCode, connect.CodeOf(err))
			} else {
				require.NoError(t, err)
			}
			require.ElementsMatch(t, tt.wantAttrs, attrs)
		})
	}
}

func recordSpanAttributes(t *testing.T, fn func(ctx context.Context)) []attribute.KeyValue {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	ctx, span := provider.Tracer("test").Start(context.Background(), "request")
	fn(ctx)
	span.End()

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	return ended[0].Attributes()
}

func Test_parseClientCapabilities(t *testing.T) {
	tests := []struct {
		Name         string
		Header       http.Header
		Want         ClientCapabilities
		WantError    bool
		ErrorMessage string
	}{
		{
			Name:   "empty header returns default capabilities",
			Header: http.Header{},
			Want:   ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "no Accept header returns default capabilities",
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "empty Accept header value returns default capabilities",
			Header: http.Header{
				"Accept": []string{""},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "simple Accept header without capabilities",
			Header: http.Header{
				"Accept": []string{"application/json"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with utf8 label names capability true",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "Accept header with utf8 label names capability false",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=false"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with utf8 label names capability invalid value",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=invalid"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with unknown capability",
			Header: http.Header{
				"Accept": []string{"*/*; unknown-capability=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
		{
			Name: "Accept header with multiple capabilities",
			Header: http.Header{
				"Accept": []string{"*/*; allow-utf8-labelnames=true; unknown-capability=false"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "multiple Accept header values",
			Header: http.Header{
				"Accept": []string{"application/json", "*/*; allow-utf8-labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "multiple Accept header values with different capabilities",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=false",
					"*/*; allow-utf8-labelnames=true",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "Accept header with quality values",
			Header: http.Header{
				"Accept": []string{"text/html; q=0.9; allow-utf8-labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "complex Accept header",
			Header: http.Header{
				"Accept": []string{
					"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8;allow-utf8-labelnames=true",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "multiple Accept header entries",
			Header: http.Header{
				"Accept": []string{
					"application/json",
					"text/plain; allow-utf8-labelnames=true",
					"*/*; q=0.1",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "invalid mime type in Accept header",
			Header: http.Header{
				"Accept": []string{"invalid/mime/type/format"},
			},
			Want:         ClientCapabilities{},
			WantError:    true,
			ErrorMessage: "mime: unexpected content after media subtype",
		},
		{
			Name: "Accept header with invalid syntax",
			Header: http.Header{
				"Accept": []string{"text/html; invalid-parameter-syntax"},
			},
			Want:         ClientCapabilities{},
			WantError:    true,
			ErrorMessage: "mime: invalid media parameter",
		},
		{
			Name: "mixed valid and invalid Accept header values",
			Header: http.Header{
				"Accept": []string{
					"application/json",
					"invalid/mime/type/format",
				},
			},
			Want:         ClientCapabilities{},
			WantError:    true,
			ErrorMessage: "mime: unexpected content after media subtype",
		},
		{
			// Parameter names are case-insensitive in mime.ParseMediaType
			Name: "case sensitivity test for capability name",
			Header: http.Header{
				"Accept": []string{"*/*; Allow-Utf8-Labelnames=true"},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "whitespace handling in Accept header",
			Header: http.Header{
				"Accept": []string{" application/json ; allow-utf8-labelnames=true "},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			got, err := parseClientCapabilities(tt.Header)

			if tt.WantError {
				require.Error(t, err)
				if tt.ErrorMessage != "" {
					require.Contains(t, err.Error(), tt.ErrorMessage)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.Want, got)
		})
	}
}

func Test_parseClientCapabilities_MultipleCapabilities(t *testing.T) {
	// This test specifically checks that when the same capability appears
	// multiple times with different values, the last "true" value wins
	tests := []struct {
		Name   string
		Header http.Header
		Want   ClientCapabilities
	}{
		{
			Name: "capability appears multiple times - last true wins",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=false",
					"text/plain; allow-utf8-labelnames=true",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "capability appears multiple times - last false loses to earlier true",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=true",
					"text/plain; allow-utf8-labelnames=false",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: true},
		},
		{
			Name: "capability appears multiple times - all false",
			Header: http.Header{
				"Accept": []string{
					"application/json; allow-utf8-labelnames=false",
					"text/plain; allow-utf8-labelnames=false",
				},
			},
			Want: ClientCapabilities{AllowUtf8LabelNames: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			got, err := parseClientCapabilities(tt.Header)
			require.NoError(t, err)
			require.Equal(t, tt.Want, got)
		})
	}
}
