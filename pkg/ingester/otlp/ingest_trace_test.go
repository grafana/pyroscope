package otlp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	profilesv1 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	distributormodel "github.com/grafana/pyroscope/v2/pkg/distributor/model"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/util/tracecontext"
)

type traceCaptureService struct {
	captured chan capturedTraceContext
}

type capturedTraceContext struct {
	upstream string
	active   trace.SpanContext
	tenantID string
}

func (s *traceCaptureService) PushBatch(ctx context.Context, _ *distributormodel.PushRequest) error {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return err
	}
	s.captured <- capturedTraceContext{
		upstream: tracecontext.UpstreamTraceID(ctx),
		active:   trace.SpanContextFromContext(ctx),
		tenantID: tenantID,
	}
	return nil
}

func TestIngest_PreservesUpstreamTraceContext(t *testing.T) {
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	svc := &traceCaptureService{captured: make(chan capturedTraceContext, 1)}
	h := NewOTLPIngestHandler(testConfig(), svc, log.NewNopLogger(), defaultLimits())
	server := httptest.NewUnstartedServer(tracecontext.HTTPMiddleware().Wrap(middleware.Tracer{}.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(tenant.InjectTenantID(r.Context(), "tenant")))
	}))))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	conn, err := grpc.NewClient(server.Listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	grpcClient := profilesv1.NewProfilesServiceClient(conn)

	// Exercise the OTLP gRPC server without any HTTP middleware as well. Its
	// tracing stats handler runs before the upstream-trace unary interceptor.
	cfg := testConfig()
	cfg.GRPCOptions = append(cfg.GRPCOptions,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(tenant.InjectTenantID(ctx, "tenant"), req)
		}),
	)
	nativeServer := newGrpcServer(cfg)
	profilesv1.RegisterProfilesServiceServer(nativeServer, h)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveErr := make(chan error, 1)
	go func() { serveErr <- nativeServer.Serve(listener) }()
	t.Cleanup(func() {
		nativeServer.Stop()
		if err := <-serveErr; err != nil {
			require.ErrorIs(t, err, grpc.ErrServerStopped)
		}
	})
	nativeConn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, nativeConn.Close()) })
	nativeClient := profilesv1.NewProfilesServiceClient(nativeConn)

	const callerTraceID = "0123456789abcdef0123456789abcdef"
	for _, protocol := range []string{"http", "grpc", "grpc-native"} {
		for _, tt := range []struct {
			name        string
			traceparent string
			want        string
		}{
			{name: "without upstream trace"},
			{name: "with upstream trace", traceparent: "00-" + callerTraceID + "-0123456789abcdef-01", want: callerTraceID},
			{name: "invalid upstream trace", traceparent: "invalid"},
		} {
			t.Run(protocol+"/"+tt.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				input := createValidOTLPRequest()
				if protocol != "http" {
					if tt.traceparent != "" {
						ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", tt.traceparent)
					}
					client := grpcClient
					if protocol == "grpc-native" {
						client = nativeClient
					}
					_, err := client.Export(ctx, input)
					require.NoError(t, err)
				} else {
					body, err := proto.Marshal(input)
					require.NoError(t, err)
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1development/profiles", bytes.NewReader(body))
					require.NoError(t, err)
					req.Header.Set("Content-Type", "application/x-protobuf")
					req.Header.Set("Traceparent", tt.traceparent)
					resp, err := server.Client().Do(req)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					require.Equal(t, http.StatusOK, resp.StatusCode)
				}
				got := <-svc.captured
				assert.Equal(t, tt.want, got.upstream)
				assert.True(t, got.active.IsValid(), "the locally created trace must remain active")
				assert.Equal(t, "tenant", got.tenantID)
			})
		}
	}
}
