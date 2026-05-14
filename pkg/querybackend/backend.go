package querybackend

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

type Config struct {
	Address          string            `yaml:"address" category:"advanced"`
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between the query-frontends and the query-schedulers."`
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

	// Capture the node type before merge() sets QueryPlan to nil.
	root := req.QueryPlan.Root
	nodeType := root.Type

	switch nodeType {
	case queryv1.QueryNode_MERGE:
		resp, childNodes, err = q.merge(ctx, req, root.Children, collectDiag)
	case queryv1.QueryNode_READ:
		resp, err = q.read(ctx, req, root.Blocks)
	default:
		panic("query plan: unknown node type")
	}

	if err != nil {
		return nil, err
	}

	if collectDiag {
		// For READ nodes, BlockReader already set the ExecutionNode with stats.
		// We just need to wrap it for MERGE nodes.
		if nodeType == queryv1.QueryNode_MERGE {
			execNode := &queryv1.ExecutionNode{
				Type:        nodeType,
				Executor:    q.hostname,
				StartTimeNs: startTime.UnixNano(),
				EndTimeNs:   time.Now().UnixNano(),
				Children:    childNodes,
			}
			if resp.Diagnostics == nil {
				resp.Diagnostics = &queryv1.Diagnostics{}
			}
			resp.Diagnostics.ExecutionNode = execNode
		}
	}

	return resp, nil
}

func (q *QueryBackend) merge(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	children []*queryv1.QueryNode,
	collectDiag bool,
) (*queryv1.InvokeResponse, []*queryv1.ExecutionNode, error) {
	request.QueryPlan = nil
	m := newAggregator(request)
	g, ctx := errgroup.WithContext(ctx)

	childExecNodes := make([]*queryv1.ExecutionNode, len(children))
	var mu sync.Mutex

	for i, child := range children {
		idx := i
		req := request.CloneVT()
		req.QueryPlan = &queryv1.QueryPlan{
			Root: child,
		}
		g.Go(util.RecoverPanic(func() error {
			// TODO: Speculative retry.
			resp, err := q.backendClient.Invoke(ctx, req)
			if err != nil {
				return err
			}
			if collectDiag && resp.Diagnostics != nil && resp.Diagnostics.ExecutionNode != nil {
				mu.Lock()
				childExecNodes[idx] = resp.Diagnostics.ExecutionNode
				mu.Unlock()
			}
			return m.aggregateResponse(resp, nil)
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	var executionNodes []*queryv1.ExecutionNode
	for _, n := range childExecNodes {
		if n != nil {
			executionNodes = append(executionNodes, n)
		}
	}

	resp := m.response()
	return resp, executionNodes, nil
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

// streamEventSender is implemented by components (BlockReader and the gRPC
// client) that can execute an InvokeRequest and forward events to a callback.
type streamEventSender interface {
	InvokeStreamEvents(ctx context.Context, req *queryv1.InvokeRequest, send func(*queryv1.InvokeStreamEvent) error) error
}

// InvokeStream implements the gRPC server-streaming RPC.
func (q *QueryBackend) InvokeStream(req *queryv1.InvokeRequest, stream queryv1.QueryBackendService_InvokeStreamServer) error {
	ctx := stream.Context()
	root := req.QueryPlan.Root

	send := func(e *queryv1.InvokeStreamEvent) error { return stream.Send(e) }

	switch root.Type {
	case queryv1.QueryNode_MERGE:
		return q.mergeStream(ctx, req, root.Children, send)
	case queryv1.QueryNode_READ:
		return q.readStream(ctx, req, root.Blocks, send)
	default:
		return status.Errorf(codes.Unknown, "query plan: unknown node type")
	}
}

func (q *QueryBackend) mergeStream(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	children []*queryv1.QueryNode,
	send func(*queryv1.InvokeStreamEvent) error,
) error {
	sender, ok := q.backendClient.(streamEventSender)
	if !ok {
		return status.Errorf(codes.Unimplemented, "backend client does not support streaming")
	}

	request.QueryPlan = nil
	var sendMu sync.Mutex
	safeSend := func(e *queryv1.InvokeStreamEvent) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return send(e)
	}

	var blocksDone, datasetsDone uint32
	var bytesDone uint64

	// Per-child latest reports. Child snapshots are cumulative full-state
	// reports (each one is the running merge of every block that child has
	// processed so far), not deltas. Aggregating them additively would
	// over-count every block once per snapshot it appears in. Instead, hold
	// each child's most recent reports and rebuild a fresh merge aggregator
	// on every parent snapshot tick.
	type childState struct {
		mu      sync.Mutex
		reports []*queryv1.Report
	}
	childStates := make([]*childState, len(children))
	for i := range childStates {
		childStates[i] = &childState{}
	}

	buildAggregate := func() *reportAggregator {
		m := newAggregator(request)
		for _, cs := range childStates {
			cs.mu.Lock()
			reports := cs.reports
			cs.mu.Unlock()
			if len(reports) == 0 {
				continue
			}
			// Errors here would indicate corrupt tree bytes from a child;
			// log-and-continue keeps streaming alive. The final aggregate
			// after the errgroup joins will surface any persistent failure.
			_ = m.aggregateResponse(&queryv1.InvokeResponse{Reports: reports}, nil)
		}
		return m
	}

	// Periodic snapshot emission.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	snapshotDone := make(chan struct{})
	go func() {
		defer close(snapshotDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap := buildAggregate().snapshot()
				if len(snap.Reports) == 0 {
					continue
				}
				_ = safeSend(&queryv1.InvokeStreamEvent{
					Event: &queryv1.InvokeStreamEvent_Snapshot{
						Snapshot: &queryv1.SnapshotEvent{
							BlocksDone:   atomic.LoadUint32(&blocksDone),
							DatasetsDone: atomic.LoadUint32(&datasetsDone),
							BytesDone:    atomic.LoadUint64(&bytesDone),
							Reports:      snap.Reports,
						},
					},
				})
			}
		}
	}()

	g, ctx2 := errgroup.WithContext(ctx)
	for i, child := range children {
		idx := i
		req := request.CloneVT()
		req.QueryPlan = &queryv1.QueryPlan{Root: child}
		g.Go(util.RecoverPanic(func() error {
			return sender.InvokeStreamEvents(ctx2, req, func(e *queryv1.InvokeStreamEvent) error {
				switch ev := e.Event.(type) {
				case *queryv1.InvokeStreamEvent_IndexLookup:
					atomic.AddUint32(&blocksDone, 1)
					atomic.AddUint32(&datasetsDone, ev.IndexLookup.DatasetsFound)
					atomic.AddUint64(&bytesDone, ev.IndexLookup.BytesEstimate)
					return safeSend(e)
				case *queryv1.InvokeStreamEvent_Snapshot:
					cs := childStates[idx]
					cs.mu.Lock()
					cs.reports = ev.Snapshot.Reports
					cs.mu.Unlock()
					return nil
				case *queryv1.InvokeStreamEvent_Terminal:
					cs := childStates[idx]
					cs.mu.Lock()
					cs.reports = ev.Terminal.Reports
					cs.mu.Unlock()
					return nil
				}
				return nil
			})
		}))
	}

	err := g.Wait()
	cancel()
	<-snapshotDone
	if err != nil {
		return err
	}

	// Build the final aggregate from the latest reports each child settled on.
	// The errgroup has joined and the snapshot goroutine has exited, so no
	// other goroutine is touching childStates.
	finalAgg := newAggregator(request)
	for _, cs := range childStates {
		if len(cs.reports) == 0 {
			continue
		}
		if err := finalAgg.aggregateResponse(&queryv1.InvokeResponse{Reports: cs.reports}, nil); err != nil {
			return err
		}
	}
	resp := finalAgg.response()
	return send(&queryv1.InvokeStreamEvent{
		Event: &queryv1.InvokeStreamEvent_Terminal{
			Terminal: &queryv1.TerminalEvent{Reports: resp.Reports},
		},
	})
}

func (q *QueryBackend) readStream(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	blocks []*metastorev1.BlockMeta,
	send func(*queryv1.InvokeStreamEvent) error,
) error {
	sender, ok := q.blockReader.(streamEventSender)
	if !ok {
		return status.Errorf(codes.Unimplemented, "block reader does not support streaming")
	}
	request.QueryPlan = &queryv1.QueryPlan{
		Root: &queryv1.QueryNode{
			Blocks: blocks,
		},
	}
	return sender.InvokeStreamEvents(ctx, request, send)
}
