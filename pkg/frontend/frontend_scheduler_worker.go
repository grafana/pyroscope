// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/frontend/v2/frontend_scheduler_worker.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package frontend

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"

	"github.com/grafana/phlare/pkg/frontend/frontendpb"
	"github.com/grafana/phlare/pkg/scheduler/schedulerdiscovery"
	"github.com/grafana/phlare/pkg/scheduler/schedulerpb"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
	"github.com/grafana/phlare/pkg/util/servicediscovery"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	schedulerAddressLabel = "scheduler_address"
	// schedulerWorkerCancelChanCapacity should be at least as big as the number of sub-queries issued by a single query
	// per scheduler (after splitting and sharding) in order to allow all of them being canceled while scheduler worker is busy.
	schedulerWorkerCancelChanCapacity = 1000
)

type frontendSchedulerWorkers struct {
	services.Service

	cfg             Config
	log             log.Logger
	frontendAddress string

	// Channel with requests that should be forwarded to the scheduler.
	requestsCh <-chan *frontendRequest

	schedulerDiscovery        services.Service
	schedulerDiscoveryWatcher *services.FailureWatcher

	mu sync.Mutex
	// Set to nil when stop is called... no more workers are created afterwards.
	workers map[string]*frontendSchedulerWorker

	enqueuedRequests *prometheus.CounterVec
}

func newFrontendSchedulerWorkers(cfg Config, frontendAddress string, requestsCh <-chan *frontendRequest, log log.Logger, reg prometheus.Registerer) (*frontendSchedulerWorkers, error) {
	f := &frontendSchedulerWorkers{
		cfg:                       cfg,
		log:                       log,
		frontendAddress:           frontendAddress,
		requestsCh:                requestsCh,
		workers:                   map[string]*frontendSchedulerWorker{},
		schedulerDiscoveryWatcher: services.NewFailureWatcher(),
		enqueuedRequests: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "phlare_query_frontend_workers_enqueued_requests_total",
			Help: "Total number of requests enqueued by each query frontend worker (regardless of the result), labeled by scheduler address.",
		}, []string{schedulerAddressLabel}),
	}

	var err error
	f.schedulerDiscovery, err = schedulerdiscovery.New(cfg.QuerySchedulerDiscovery, cfg.SchedulerAddress, cfg.DNSLookupPeriod, "query-frontend", f, log, reg)
	if err != nil {
		return nil, err
	}

	f.Service = services.NewBasicService(f.starting, f.running, f.stopping)
	return f, nil
}

func (f *frontendSchedulerWorkers) starting(ctx context.Context) error {
	f.schedulerDiscoveryWatcher.WatchService(f.schedulerDiscovery)

	return services.StartAndAwaitRunning(ctx, f.schedulerDiscovery)
}

func (f *frontendSchedulerWorkers) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-f.schedulerDiscoveryWatcher.Chan():
		return errors.Wrap(err, "query-frontend workers subservice failed")
	}
}

func (f *frontendSchedulerWorkers) stopping(_ error) error {
	err := services.StopAndAwaitTerminated(context.Background(), f.schedulerDiscovery)

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, w := range f.workers {
		w.stop()
	}
	f.workers = nil

	return err
}

func (f *frontendSchedulerWorkers) InstanceAdded(instance servicediscovery.Instance) {
	// Connect only to in-use query-scheduler instances.
	if instance.InUse {
		f.addScheduler(instance.Address)
	}
}

func (f *frontendSchedulerWorkers) addScheduler(address string) {
	f.mu.Lock()
	ws := f.workers
	w := f.workers[address]

	// Already stopped or we already have worker for this address.
	if ws == nil || w != nil {
		f.mu.Unlock()
		return
	}
	f.mu.Unlock()

	level.Info(f.log).Log("msg", "adding connection to query-scheduler", "addr", address)
	conn, err := f.connectToScheduler(context.Background(), address)
	if err != nil {
		level.Error(f.log).Log("msg", "error connecting to query-scheduler", "addr", address, "err", err)
		return
	}

	// No worker for this address yet, start a new one.
	w = newFrontendSchedulerWorker(conn, address, f.frontendAddress, f.requestsCh, f.cfg.WorkerConcurrency, f.enqueuedRequests.WithLabelValues(address), f.cfg.MaxLoopDuration, f.log)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Can be nil if stopping has been called already.
	if f.workers == nil {
		return
	}
	// We have to recheck for presence in case we got called again while we were
	// connecting and that one finished first.
	if f.workers[address] != nil {
		return
	}
	f.workers[address] = w
	w.start()
}

func (f *frontendSchedulerWorkers) InstanceRemoved(instance servicediscovery.Instance) {
	f.removeScheduler(instance.Address)
}

func (f *frontendSchedulerWorkers) removeScheduler(address string) {
	f.mu.Lock()
	// This works fine if f.workers is nil already or the worker is missing
	// because the query-scheduler instance was not in use.
	w := f.workers[address]
	delete(f.workers, address)
	f.mu.Unlock()

	if w != nil {
		level.Info(f.log).Log("msg", "removing connection to query-scheduler", "addr", address)
		w.stop()
	}
	f.enqueuedRequests.Delete(prometheus.Labels{schedulerAddressLabel: address})
}

func (f *frontendSchedulerWorkers) InstanceChanged(instance servicediscovery.Instance) {
	// Ensure the query-frontend connects to in-use query-scheduler instances and disconnect from ones no more in use.
	// The called methods correctly handle the case the query-frontend is already connected/disconnected
	// to/from the given query-scheduler instance.
	if instance.InUse {
		f.addScheduler(instance.Address)
	} else {
		f.removeScheduler(instance.Address)
	}
}

// Get number of workers.
func (f *frontendSchedulerWorkers) getWorkersCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.workers)
}

func (f *frontendSchedulerWorkers) connectToScheduler(ctx context.Context, address string) (*grpc.ClientConn, error) {
	// Because we only use single long-running method, it doesn't make sense to inject user ID, send over tracing or add metrics.
	opts, err := f.cfg.GRPCClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Worker managing single gRPC connection to Scheduler. Each worker starts multiple goroutines for forwarding
// requests and cancellations to scheduler.
type frontendSchedulerWorker struct {
	log log.Logger

	conn          *grpc.ClientConn
	concurrency   int
	schedulerAddr string
	frontendAddr  string

	// Context and cancellation used by individual goroutines.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Shared between all frontend workers.
	requestCh <-chan *frontendRequest

	// Cancellation requests for this scheduler are received via this channel. It is passed to frontend after
	// query has been enqueued to scheduler.
	cancelCh chan uint64

	// Number of queries sent to this scheduler.
	enqueuedRequests prometheus.Counter

	maxLoopDuration time.Duration
}

func newFrontendSchedulerWorker(conn *grpc.ClientConn, schedulerAddr string, frontendAddr string, requestCh <-chan *frontendRequest, concurrency int, enqueuedRequests prometheus.Counter, maxLoopDuration time.Duration, log log.Logger) *frontendSchedulerWorker {
	w := &frontendSchedulerWorker{
		log:              log,
		conn:             conn,
		concurrency:      concurrency,
		schedulerAddr:    schedulerAddr,
		frontendAddr:     frontendAddr,
		requestCh:        requestCh,
		cancelCh:         make(chan uint64, schedulerWorkerCancelChanCapacity),
		enqueuedRequests: enqueuedRequests,
		maxLoopDuration:  maxLoopDuration,
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())

	return w
}

func (w *frontendSchedulerWorker) start() {
	client := schedulerpb.NewSchedulerForFrontendClient(w.conn)
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.runOne(w.ctx, client)
		}()
	}
}

func (w *frontendSchedulerWorker) stop() {
	w.cancel()
	w.wg.Wait()
	if err := w.conn.Close(); err != nil {
		level.Error(w.log).Log("msg", "error while closing connection to scheduler", "err", err)
	}
}

func (w *frontendSchedulerWorker) runOne(ctx context.Context, client schedulerpb.SchedulerForFrontendClient) {
	// attemptLoop returns false if there was any error with forwarding requests to scheduler.
	attemptLoop := func() bool {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel() // cancel the stream after we are done to release resources

		loop, loopErr := client.FrontendLoop(ctx)
		if loopErr != nil {
			level.Error(w.log).Log("msg", "error contacting scheduler", "err", loopErr, "addr", w.schedulerAddr)
			return false
		}

		loopErr = w.schedulerLoop(loop)
		if loopErr == io.EOF {
			level.Debug(w.log).Log("msg", "scheduler loop closed", "addr", w.schedulerAddr)
			return true
		}
		if closeErr := loop.CloseSend(); closeErr != nil {
			level.Debug(w.log).Log("msg", "failed to close frontend loop", "err", loopErr, "addr", w.schedulerAddr)
		}

		if loopErr != nil {
			level.Error(w.log).Log("msg", "error sending requests to scheduler", "err", loopErr, "addr", w.schedulerAddr)
			return false
		}
		return true
	}

	backoffConfig := backoff.Config{
		MinBackoff: 250 * time.Millisecond,
		MaxBackoff: 2 * time.Second,
	}
	backoff := backoff.New(ctx, backoffConfig)
	for backoff.Ongoing() {
		if !attemptLoop() {
			backoff.Wait()
		} else {
			backoff.Reset()
		}
	}
}

func jitter(d time.Duration, factor float64) time.Duration {
	maxJitter := time.Duration(float64(d) * factor)
	return d - time.Duration(rand.Int63n(int64(maxJitter)))
}

func (w *frontendSchedulerWorker) schedulerLoop(loop schedulerpb.SchedulerForFrontend_FrontendLoopClient) error {
	if err := loop.Send(&schedulerpb.FrontendToScheduler{
		Type:            schedulerpb.FrontendToSchedulerType_INIT,
		FrontendAddress: w.frontendAddr,
	}); err != nil {
		return err
	}

	if resp, err := loop.Recv(); err != nil || resp.Status != schedulerpb.SchedulerToFrontendStatus_OK {
		if err != nil {
			return err
		}
		return errors.Errorf("unexpected status received for init: %v", resp.Status)
	}

	ctx, cancel := context.WithCancel(loop.Context())
	defer cancel()
	if w.maxLoopDuration > 0 {
		go func() {
			timer := time.NewTimer(jitter(w.maxLoopDuration, 0.3))
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				cancel()
				return
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			// No need to report error if our internal context is canceled. This can happen during shutdown,
			// or when scheduler is no longer resolvable. (It would be nice if this context reported "done" also when
			// connection scheduler stops the call, but that doesn't seem to be the case).
			//
			// Reporting error here would delay reopening the stream (if the worker context is not done yet).
			level.Debug(w.log).Log("msg", "stream context finished", "err", ctx.Err())
			return nil
		case req := <-w.requestCh:
			err := loop.Send(&schedulerpb.FrontendToScheduler{
				Type:            schedulerpb.FrontendToSchedulerType_ENQUEUE,
				QueryID:         req.queryID,
				UserID:          req.userID,
				HttpRequest:     req.request,
				FrontendAddress: w.frontendAddr,
				StatsEnabled:    req.statsEnabled,
			})
			w.enqueuedRequests.Inc()

			if err != nil {
				req.enqueue <- enqueueResult{status: failed}
				return err
			}

			resp, err := loop.Recv()
			if err != nil {
				req.enqueue <- enqueueResult{status: failed}
				return err
			}

			switch resp.Status {
			case schedulerpb.SchedulerToFrontendStatus_OK:
				req.enqueue <- enqueueResult{status: waitForResponse, cancelCh: w.cancelCh}
				// Response will come from querier.

			case schedulerpb.SchedulerToFrontendStatus_SHUTTING_DOWN:
				// Scheduler is shutting down, report failure to enqueue and stop this loop.
				req.enqueue <- enqueueResult{status: failed}
				return errors.New("scheduler is shutting down")

			case schedulerpb.SchedulerToFrontendStatus_ERROR:
				req.enqueue <- enqueueResult{status: waitForResponse}
				req.response <- &frontendpb.QueryResultRequest{
					HttpResponse: &httpgrpc.HTTPResponse{
						Code: http.StatusInternalServerError,
						Body: []byte(err.Error()),
					},
				}

			case schedulerpb.SchedulerToFrontendStatus_TOO_MANY_REQUESTS_PER_TENANT:
				req.enqueue <- enqueueResult{status: waitForResponse}
				req.response <- &frontendpb.QueryResultRequest{
					HttpResponse: &httpgrpc.HTTPResponse{
						Code: http.StatusTooManyRequests,
						Body: []byte("too many outstanding requests"),
					},
				}

			default:
				level.Error(w.log).Log("msg", "unknown response status from the scheduler", "resp", resp, "queryID", req.queryID)
				req.enqueue <- enqueueResult{status: failed}
			}

		case reqID := <-w.cancelCh:
			err := loop.Send(&schedulerpb.FrontendToScheduler{
				Type:    schedulerpb.FrontendToSchedulerType_CANCEL,
				QueryID: reqID,
			})
			if err != nil {
				return err
			}

			resp, err := loop.Recv()
			if err != nil {
				return err
			}

			// Scheduler may be shutting down, report that.
			if resp.Status != schedulerpb.SchedulerToFrontendStatus_OK {
				return errors.Errorf("unexpected status received for cancellation: %v", resp.Status)
			}
		}
	}
}
