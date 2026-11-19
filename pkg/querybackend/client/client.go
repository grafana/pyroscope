package querybackendclient

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/stats"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/internal/pushback"
)

type Client struct {
	service    services.Service
	grpcClient queryv1.QueryBackendServiceClient
	conn       *grpc.ClientConn
}

func New(address string, grpcClientConfig grpcclient.Config, dialOpts ...grpc.DialOption) (*Client, error) {
	conn, err := dial(address, grpcClientConfig, dialOpts...)
	if err != nil {
		return nil, err
	}
	var c Client
	c.conn = conn
	c.grpcClient = queryv1.NewQueryBackendServiceClient(conn)
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func dial(address string, grpcClientConfig grpcclient.Config, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	options, err := grpcClientConfig.DialOption(nil, nil, nil)
	if err != nil {
		return nil, err
	}
	options = append(options,
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
		grpc.WithMaxCallAttempts(500),
		grpc.WithStatsHandler(retryStatsHandler{}),
	)
	if strings.HasPrefix(address, "dns:") {
		options = append(options, grpc.WithResolvers(refreshingDNSBuilder{delegate: resolver.Get("dns")}))
	}
	options = append(options, dialOpts...)
	return grpc.NewClient(address, options...)
}

func (b *Client) Service() services.Service      { return b.service }
func (b *Client) starting(context.Context) error { return nil }
func (b *Client) stopping(error) error           { return nil }
func (b *Client) Close() error                   { return b.conn.Close() }

func (b *Client) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	ctx, tracker := withRetryTracker(ctx)
	var trailer metadata.MD
	start := time.Now()
	resp, err := b.grpcClient.Invoke(ctx, req, grpc.Trailer(&trailer))
	elapsed := time.Since(start).Nanoseconds()
	if err != nil && pushback.IsNoRetry(trailer) {
		err = pushback.Mark(err)
	}
	if err == nil && resp != nil {
		if resp.Diagnostics == nil {
			resp.Diagnostics = &queryv1.Diagnostics{}
		}
		if resp.Diagnostics.ExecutionNode == nil {
			resp.Diagnostics.ExecutionNode = &queryv1.ExecutionNode{}
		}
		if resp.Diagnostics.ExecutionNode.Stats == nil {
			resp.Diagnostics.ExecutionNode.Stats = &queryv1.ExecutionStats{}
		}
		stats := resp.Diagnostics.ExecutionNode.Stats
		stats.BackoffDurationNs += tracker.backoffDuration().Nanoseconds()
		// Preserve the backend's processing breakdown. Transport, connection
		// waits and failed attempts cannot be classified from the response.
		accounted := stats.ProcessingDurationNs + stats.BackoffDurationNs + stats.UnaccountedDurationNs
		if remaining := elapsed - accounted; remaining > 0 {
			stats.UnaccountedDurationNs += remaining
		}
	}
	return resp, err
}

type retryTrackerKey struct{}

type retryTracker struct {
	mu          sync.Mutex
	lastAttempt time.Time
	backoff     time.Duration
}

func withRetryTracker(ctx context.Context) (context.Context, *retryTracker) {
	t := &retryTracker{}
	return context.WithValue(ctx, retryTrackerKey{}, t), t
}

func (t *retryTracker) onBegin(beginTime time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.lastAttempt.IsZero() && beginTime.After(t.lastAttempt) {
		t.backoff += beginTime.Sub(t.lastAttempt)
	}
}

func (t *retryTracker) onEnd(endTime time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastAttempt = endTime
}

func (t *retryTracker) backoffDuration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.backoff
}

type retryStatsHandler struct{}

func (retryStatsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (retryStatsHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	t, _ := ctx.Value(retryTrackerKey{}).(*retryTracker)
	if t == nil {
		return
	}
	switch s := rs.(type) {
	case *stats.Begin:
		if s.IsClient() {
			t.onBegin(s.BeginTime)
		}
	case *stats.End:
		if s.IsClient() {
			t.onEnd(s.EndTime)
		}
	}
}

func (retryStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (retryStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

const grpcServiceConfig = `{
    "loadBalancingPolicy":"round_robin",
    "methodConfig": [{
        "name": [{"service": ""}],
        "waitForReady": true,
        "retryPolicy": {
            "MaxAttempts": 500,
            "InitialBackoff": "1s",
            "MaxBackoff": "2s",
            "BackoffMultiplier": 1.1,
            "RetryableStatusCodes": [
              "UNAVAILABLE",
              "RESOURCE_EXHAUSTED"
            ]
        }
    }]
}`
