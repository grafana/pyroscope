package querybackend

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/internal/pushback"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type Config struct {
	Address          string            `yaml:"address" category:"advanced"`
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate with query-backends. backoff_on_ratelimits is ignored: its retries ignore the server's pushback."`
	ClientTimeout    time.Duration     `yaml:"client_timeout" category:"advanced"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.Address, "query-backend.address", "localhost:9095", "")
	f.DurationVar(&cfg.ClientTimeout, "query-backend.client-timeout", 30*time.Second, "Timeout for query-backend client requests.")
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("query-backend.grpc-client-config", f)
}

func (cfg *Config) Validate() error {
	if cfg.Address == "" {
		return fmt.Errorf("query-backend.address is required")
	}
	return cfg.GRPCClientConfig.Validate()
}

// DisableClientRateLimitRetries turns off dskit's retrier, which ignores grpc-retry-pushback-ms.
func (cfg *Config) DisableClientRateLimitRetries(logger log.Logger) {
	if !cfg.GRPCClientConfig.BackoffOnRatelimits {
		return
	}
	level.Warn(logger).Log("msg", "ignoring query-backend.grpc-client-config.backoff-on-ratelimits: its retries ignore grpc-retry-pushback-ms")
	cfg.GRPCClientConfig.BackoffOnRatelimits = false
}

type QueryHandler interface {
	Invoke(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type QueryBackend struct {
	service services.Service
	queryv1.QueryBackendServiceServer

	config Config
	logger log.Logger
	reg    prometheus.Registerer

	backendClient QueryHandler
	blockReader   QueryHandler
	hostname      string
}

func New(
	config Config,
	logger log.Logger,
	reg prometheus.Registerer,
	backendClient QueryHandler,
	blockReader QueryHandler,
) (*QueryBackend, error) {
	hostname, _ := os.Hostname()
	q := QueryBackend{
		config:        config,
		logger:        logger,
		reg:           reg,
		backendClient: backendClient,
		blockReader:   blockReader,
		hostname:      hostname,
	}
	q.service = services.NewIdleService(q.starting, q.stopping)
	return &q, nil
}

func (q *QueryBackend) Service() services.Service      { return q.service }
func (q *QueryBackend) starting(context.Context) error { return nil }
func (q *QueryBackend) stopping(error) error           { return nil }

func (q *QueryBackend) Invoke(
	ctx context.Context,
	req *queryv1.InvokeRequest,
) (*queryv1.InvokeResponse, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryBackend.Invoke")
	defer span.Finish()

	collectDiag := req.Options != nil && req.Options.CollectDiagnostics
	startTime := time.Now()

	var resp *queryv1.InvokeResponse
	var err error
	var childNodes []*queryv1.ExecutionNode
	var mergeStats *queryv1.ExecutionStats

	// Capture the node type before merge() sets QueryPlan to nil.
	root := req.QueryPlan.Root
	nodeType := root.Type

	switch nodeType {
	case queryv1.QueryNode_MERGE:
		resp, childNodes, mergeStats, err = q.merge(ctx, req, root.Children, collectDiag)
	case queryv1.QueryNode_READ:
		resp, err = q.read(ctx, req, root.Blocks)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("query plan: unknown node type %v", nodeType))
	}

	if err != nil {
		// A child cannot deliver its response; neither can we.
		if pushback.IsMarked(err) {
			pushback.SetNoRetry(ctx)
		}
		return nil, err
	}

	// Only the root invocation has the complete result. Intermediate nodes
	// must retain their series without running detection or limiting events.
	if req.GetOptions().GetFinalize() {
		for _, report := range resp.Reports {
			if report.TimeSeriesAnalysis != nil {
				if err := finalizeTimeSeriesAnalysis(report.TimeSeriesAnalysis); err != nil {
					return nil, err
				}
			}
		}
	}

	// For MERGE nodes, unconditionally expose the summed BytesFetched and
	// critical-path duration breakdown so the query-frontend metrics are
	// correct even when diagnostics are not collected (the common production
	// case). For READ nodes, BlockReader already set the ExecutionNode.
	if nodeType == queryv1.QueryNode_MERGE {
		if resp.Diagnostics == nil {
			resp.Diagnostics = &queryv1.Diagnostics{}
		}
		endTime := time.Now()
		// Add time spent after child RPCs completed (e.g. final response building).
		mergeDurationNs := endTime.Sub(startTime).Nanoseconds()
		if childTotalNs := mergeStats.ProcessingDurationNs + mergeStats.BackoffDurationNs + mergeStats.UnaccountedDurationNs; mergeDurationNs > childTotalNs {
			mergeStats.ProcessingDurationNs += mergeDurationNs - childTotalNs
		}
		if collectDiag {
			resp.Diagnostics.ExecutionNode = &queryv1.ExecutionNode{
				Type:        nodeType,
				Executor:    q.hostname,
				StartTimeNs: startTime.UnixNano(),
				EndTimeNs:   endTime.UnixNano(),
				Children:    childNodes,
				Stats:       mergeStats,
			}
		} else {
			resp.Diagnostics.ExecutionNode = &queryv1.ExecutionNode{
				Stats: mergeStats,
			}
		}
	}

	if nodeType == queryv1.QueryNode_READ {
		// BlockReader snapshots its timing before root-level finalization. Include
		// all local processing here so the client does not classify it as unaccounted.
		if node := resp.GetDiagnostics().GetExecutionNode(); node != nil && node.Stats != nil {
			endTime := time.Now()
			node.Stats.ProcessingDurationNs = endTime.Sub(startTime).Nanoseconds()
			if collectDiag {
				node.StartTimeNs = startTime.UnixNano()
				node.EndTimeNs = endTime.UnixNano()
			}
		}
	}

	// The response is complete: all that remains is encoding and the size check in
	// grpc-go's Server.sendResponse, so a failure there recurs on every attempt.
	// Queries that failed return above, still retryable.
	pushback.SetNoRetry(ctx)

	return resp, nil
}

func (q *QueryBackend) merge(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	children []*queryv1.QueryNode,
	collectDiag bool,
) (*queryv1.InvokeResponse, []*queryv1.ExecutionNode, *queryv1.ExecutionStats, error) {
	request.QueryPlan = nil
	m := newAggregator(request)
	g, ctx := errgroup.WithContext(ctx)

	childExecNodes := make([]*queryv1.ExecutionNode, len(children))
	var mu sync.Mutex
	var totalBytesFetched atomic.Uint64
	var noRetry atomic.Bool
	var maxChildTotalNs int64
	var criticalProcessingNs int64
	var criticalBackoffNs int64
	var criticalUnaccountedNs int64

	for i, child := range children {
		idx := i
		req := request.CloneVT()
		if req.Options != nil {
			req.Options.Finalize = false
		}
		req.QueryPlan = &queryv1.QueryPlan{
			Root: child,
		}
		g.Go(util.RecoverPanic(func() error {
			// TODO: Speculative retry.
			childStart := time.Now()
			resp, err := q.backendClient.Invoke(ctx, req)
			if err != nil {
				if pushback.IsMarked(err) {
					noRetry.Store(true)
				}
				return err
			}
			elapsedNs := time.Since(childStart).Nanoseconds()
			childStats := resp.GetDiagnostics().GetExecutionNode().GetStats()
			// Always accumulate bytes regardless of collectDiag so the
			// query-frontend counter is correct in the common (non-diagnostic) path.
			totalBytesFetched.Add(childStats.GetBytesFetched())
			if collectDiag && resp.Diagnostics != nil && resp.Diagnostics.ExecutionNode != nil {
				mu.Lock()
				childExecNodes[idx] = resp.Diagnostics.ExecutionNode
				mu.Unlock()
			}
			childBackoffNs := childStats.GetBackoffDurationNs()
			childProcessingNs := childStats.GetProcessingDurationNs()
			childUnaccountedNs := childStats.GetUnaccountedDurationNs()
			if remaining := elapsedNs - childProcessingNs - childBackoffNs - childUnaccountedNs; remaining > 0 {
				childUnaccountedNs += remaining
			}
			aggregationStart := time.Now()
			err = m.aggregateResponse(resp, nil)
			childProcessingNs += time.Since(aggregationStart).Nanoseconds()
			childTotalNs := childProcessingNs + childBackoffNs + childUnaccountedNs
			mu.Lock()
			if childTotalNs > maxChildTotalNs {
				maxChildTotalNs = childTotalNs
				criticalProcessingNs = childProcessingNs
				criticalBackoffNs = childBackoffNs
				criticalUnaccountedNs = childUnaccountedNs
			}
			mu.Unlock()
			return err
		}))
	}
	if err := g.Wait(); err != nil {
		// errgroup keeps one error; a sibling's verdict must not be lost with it.
		if noRetry.Load() {
			err = pushback.Mark(err)
		}
		return nil, nil, nil, err
	}

	var executionNodes []*queryv1.ExecutionNode
	for _, n := range childExecNodes {
		if n != nil {
			executionNodes = append(executionNodes, n)
		}
	}

	resp := m.response()
	return resp, executionNodes, &queryv1.ExecutionStats{
		BytesFetched:          totalBytesFetched.Load(),
		ProcessingDurationNs:  criticalProcessingNs,
		BackoffDurationNs:     criticalBackoffNs,
		UnaccountedDurationNs: criticalUnaccountedNs,
	}, nil
}

func (q *QueryBackend) read(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	blocks []*metastorev1.BlockMeta,
) (*queryv1.InvokeResponse, error) {
	request.QueryPlan = &queryv1.QueryPlan{
		Root: &queryv1.QueryNode{
			Blocks: blocks,
		},
	}
	return q.blockReader.Invoke(ctx, request)
}
