package util

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

type grpcClientStatsKey struct{}

func GrpcClientStatsHandler(reg prometheus.Registerer) stats.Handler {
	return &statsHandler{
		ElapsedDuration: RegisterOrGet(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       "pyroscope",
			Subsystem:                       "grpc_client",
			Name:                            "request_duration_seconds",
			Help:                            "Time (in seconds) required to send and recieve a gRPC request/response.",
			Buckets:                         prometheus.DefBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  50,
			NativeHistogramMinResetDuration: time.Hour,
		}, []string{"method", "status_code"})),

		RequestDecompressedBytes: RegisterOrGet(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Subsystem: "grpc_client",
			Name:      "request_body_bytes",
			Help:      "Number of decompressed bytes in the request body.",
		}, []string{"method"})),

		ResponseDecompressedBytes: RegisterOrGet(reg, prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Subsystem: "grpc_client",
			Name:      "response_body_bytes",
			Help:      "Number of decompressed bytes in the response body.",
		}, []string{"method"})),
	}
}

type statsHandler struct {
	ElapsedDuration           *prometheus.HistogramVec
	RequestDecompressedBytes  *prometheus.HistogramVec
	ResponseDecompressedBytes *prometheus.HistogramVec
}

func (s *statsHandler) HandleConn(_ context.Context, _ stats.ConnStats) {}

func (s *statsHandler) HandleRPC(ctx context.Context, rpcStats stats.RPCStats) {
	if !rpcStats.IsClient() {
		return
	}

	info, ok := ctx.Value(grpcClientStatsKey{}).(*stats.RPCTagInfo)
	if !ok {
		return
	}

	switch msg := rpcStats.(type) {
	case *stats.InPayload:
		s.ResponseDecompressedBytes.With(prometheus.Labels{
			"method": info.FullMethodName,
		}).Observe(float64(msg.Length))

	case *stats.OutPayload:
		s.RequestDecompressedBytes.With(prometheus.Labels{
			"method": info.FullMethodName,
		}).Observe(float64(msg.Length))

	case *stats.End:
		statusCode, _ := status.FromError(msg.Error)

		s.ElapsedDuration.With(prometheus.Labels{
			"method":      info.FullMethodName,
			"status_code": statusCode.Code().String(),
		}).Observe(msg.EndTime.Sub(msg.BeginTime).Seconds())
	}
}

func (s *statsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (s *statsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return context.WithValue(ctx, grpcClientStatsKey{}, info)
}
