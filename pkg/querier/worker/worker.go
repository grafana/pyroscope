// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/querier/worker/worker.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package worker

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	"github.com/grafana/phlare/pkg/scheduler/schedulerdiscovery"
	"github.com/grafana/phlare/pkg/util/httpgrpc"
	"github.com/grafana/phlare/pkg/util/servicediscovery"
)

type Config struct {
	SchedulerAddress string            `yaml:"scheduler_address" doc:"hidden"`
	DNSLookupPeriod  time.Duration     `yaml:"dns_lookup_duration" category:"advanced" doc:"hidden"`
	QuerierID        string            `yaml:"id" category:"advanced"`
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between the queriers and the query-frontends / query-schedulers."`

	// This configuration is injected internally.
	MaxConcurrentRequests   int                       `yaml:"-"` // Must be same as passed to PromQL Engine.
	QuerySchedulerDiscovery schedulerdiscovery.Config `yaml:"-"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.QuerierID, "querier.id", "", "Querier ID, sent to the query-frontend to identify requests from the same querier. Defaults to hostname.")

	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("querier.frontend-client", f)
}

func (cfg *Config) Validate(log log.Logger) error {
	if cfg.QuerySchedulerDiscovery.Mode == schedulerdiscovery.ModeRing && cfg.SchedulerAddress != "" {
		return fmt.Errorf("scheduler address cannot be specified when query-scheduler service discovery mode is set to '%s'", cfg.QuerySchedulerDiscovery.Mode)
	}

	return cfg.GRPCClientConfig.Validate(log)
}

// RequestHandler for HTTP requests wrapped in protobuf messages.
type RequestHandler interface {
	Handle(context.Context, *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error)
}

// Single processor handles all streaming operations to query-frontend or query-scheduler to fetch queries
// and process them.
type processor interface {
	// Each invocation of processQueriesOnSingleStream starts new streaming operation to query-frontend
	// or query-scheduler to fetch queries and execute them.
	//
	// This method must react on context being finished, and stop when that happens.
	//
	// processorManager (not processor) is responsible for starting as many goroutines as needed for each connection.
	processQueriesOnSingleStream(ctx context.Context, conn *grpc.ClientConn, address string)

	// notifyShutdown notifies the remote query-frontend or query-scheduler that the querier is
	// shutting down.
	notifyShutdown(ctx context.Context, conn *grpc.ClientConn, address string)
}

// serviceDiscoveryFactory makes a new service discovery instance.
type serviceDiscoveryFactory func(receiver servicediscovery.Notifications) (services.Service, error)

type querierWorker struct {
	*services.BasicService

	cfg Config
	log log.Logger

	processor processor

	// Subservices manager.
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	mu        sync.Mutex
	managers  map[string]*processorManager
	instances map[string]servicediscovery.Instance
}

func NewQuerierWorker(cfg Config, handler RequestHandler, log log.Logger, reg prometheus.Registerer) (services.Service, error) {
	if cfg.QuerierID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get hostname for configuring querier ID")
		}
		cfg.QuerierID = hostname
	}

	var processor processor
	var servs []services.Service
	var factory serviceDiscoveryFactory

	switch {
	case cfg.SchedulerAddress != "" || cfg.QuerySchedulerDiscovery.Mode == schedulerdiscovery.ModeRing:
		level.Info(log).Log("msg", "Starting querier worker connected to query-scheduler", "scheduler", cfg.SchedulerAddress)

		factory = func(receiver servicediscovery.Notifications) (services.Service, error) {
			return schedulerdiscovery.New(cfg.QuerySchedulerDiscovery, cfg.SchedulerAddress, cfg.DNSLookupPeriod, "querier", receiver, log, reg)
		}

		processor, servs = newSchedulerProcessor(cfg, handler, log, reg)

	default:
		return nil, errors.New("no query-scheduler or query-frontend address")
	}

	return newQuerierWorkerWithProcessor(cfg, log, processor, factory, servs)
}

func newQuerierWorkerWithProcessor(cfg Config, log log.Logger, processor processor, newServiceDiscovery serviceDiscoveryFactory, servs []services.Service) (*querierWorker, error) {
	f := &querierWorker{
		cfg:       cfg,
		log:       log,
		managers:  map[string]*processorManager{},
		instances: map[string]servicediscovery.Instance{},
		processor: processor,
	}

	// There's no service discovery in some tests.
	if newServiceDiscovery != nil {
		w, err := newServiceDiscovery(f)
		if err != nil {
			return nil, err
		}

		servs = append(servs, w)
	}

	if len(servs) > 0 {
		subservices, err := services.NewManager(servs...)
		if err != nil {
			return nil, errors.Wrap(err, "querier worker subservices")
		}

		f.subservices = subservices
		f.subservicesWatcher = services.NewFailureWatcher()
	}

	f.BasicService = services.NewBasicService(f.starting, f.running, f.stopping)
	return f, nil
}

func (w *querierWorker) starting(ctx context.Context) error {
	if w.subservices == nil {
		return nil
	}

	w.subservicesWatcher.WatchManager(w.subservices)
	return services.StartManagerAndAwaitHealthy(ctx, w.subservices)
}

func (w *querierWorker) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-w.subservicesWatcher.Chan(): // The channel will be nil if w.subservicesWatcher is not set.
		return errors.Wrap(err, "querier worker subservice failed")
	}
}

func (w *querierWorker) stopping(_ error) error {
	// Stop all goroutines fetching queries. Note that in Stopping state,
	// worker no longer creates new managers in InstanceAdded method.
	w.mu.Lock()
	for address, m := range w.managers {
		m.stop()

		delete(w.managers, address)
		delete(w.instances, address)
	}
	w.mu.Unlock()

	if w.subservices == nil {
		return nil
	}

	// Stop service discovery and services used by processor.
	return services.StopManagerAndAwaitStopped(context.Background(), w.subservices)
}

func (w *querierWorker) InstanceAdded(instance servicediscovery.Instance) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Ensure the querier worker hasn't been stopped (or is stopping).
	// This check is done inside the lock, to avoid any race condition with the stopping() function.
	ctx := w.ServiceContext()
	if ctx == nil || ctx.Err() != nil {
		return
	}

	address := instance.Address
	if m := w.managers[address]; m != nil {
		return
	}

	level.Info(w.log).Log("msg", "adding connection", "addr", address, "in-use", instance.InUse)
	conn, err := w.connect(context.Background(), address)
	if err != nil {
		level.Error(w.log).Log("msg", "error connecting", "addr", address, "err", err)
		return
	}

	w.managers[address] = newProcessorManager(ctx, w.processor, conn, address)
	w.instances[address] = instance

	// Called with lock.
	w.resetConcurrency()
}

func (w *querierWorker) InstanceRemoved(instance servicediscovery.Instance) {
	address := instance.Address

	level.Info(w.log).Log("msg", "removing connection", "addr", address, "in-use", instance.InUse)

	w.mu.Lock()
	p := w.managers[address]
	delete(w.managers, address)
	delete(w.instances, address)
	w.mu.Unlock()

	if p != nil {
		p.stop()
	}

	// Re-balance the connections between the available query-frontends / query-schedulers.
	w.mu.Lock()
	w.resetConcurrency()
	w.mu.Unlock()
}

func (w *querierWorker) InstanceChanged(instance servicediscovery.Instance) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Ensure the querier worker hasn't been stopped (or is stopping).
	// This check is done inside the lock, to avoid any race condition with the stopping() function.
	ctx := w.ServiceContext()
	if ctx == nil || ctx.Err() != nil {
		return
	}

	// Ensure there's a manager for the instance. If there's no, then it's a bug.
	if m := w.managers[instance.Address]; m == nil {
		level.Error(w.log).Log("msg", "received a notification about an unknown backend instance", "addr", instance.Address, "in-use", instance.InUse)
		return
	}

	level.Info(w.log).Log("msg", "updating connection", "addr", instance.Address, "in-use", instance.InUse)

	// Update instance and adjust concurrency.
	w.instances[instance.Address] = instance

	// Called with lock.
	w.resetConcurrency()
}

// Must be called with lock.
func (w *querierWorker) resetConcurrency() {
	desiredConcurrency := w.getDesiredConcurrency()

	for _, m := range w.managers {
		concurrency, ok := desiredConcurrency[m.address]
		if !ok {
			// This error should never happen. If it does, it means there's a bug in the code.
			level.Error(w.log).Log("msg", "a querier worker is connected to an unknown remote endpoint", "addr", m.address)

			// Consider it as not in-use.
			concurrency = 1
		}

		m.concurrency(concurrency)
	}
}

// getDesiredConcurrency returns the number of desired connections for each discovered query-frontend / query-scheduler instance.
// Must be called with lock.
func (w *querierWorker) getDesiredConcurrency() map[string]int {
	// Count the number of in-use instances.
	numInUse := 0
	for _, instance := range w.instances {
		if instance.InUse {
			numInUse++
		}
	}

	var (
		desired    = make(map[string]int, len(w.instances))
		inUseIndex = 0
	)

	// Compute the number of desired connections for each discovered instance.
	for address, instance := range w.instances {
		// Run only 1 worker for each instance not in-use, to allow for the queues
		// to be drained when the in-use instances change or if, for any reason,
		// queries are enqueued on the ones not in-use.
		if !instance.InUse {
			desired[address] = 1
			continue
		}

		concurrency := w.cfg.MaxConcurrentRequests / numInUse

		// If max concurrency does not evenly divide into in-use instances, then a subset will be chosen
		// to receive an extra connection. Since we're iterating a map (whose iteration order is not guaranteed),
		// then this should pratically select a random address for the extra connection.
		if inUseIndex < w.cfg.MaxConcurrentRequests%numInUse {
			level.Warn(w.log).Log("msg", "max concurrency is not evenly divisible across targets, adding an extra connection", "addr", address)
			concurrency++
		}

		// If concurrency is 0 then MaxConcurrentRequests is less than the total number of
		// frontends/schedulers. In order to prevent accidentally starving a frontend or scheduler we are just going to
		// always connect once to every target.
		if concurrency == 0 {
			concurrency = 1
		}

		desired[address] = concurrency
		inUseIndex++
	}

	return desired
}

func (w *querierWorker) connect(ctx context.Context, address string) (*grpc.ClientConn, error) {
	// Because we only use single long-running method, it doesn't make sense to inject user ID, send over tracing or add metrics.
	opts, err := w.cfg.GRPCClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
