package fire

import (
	"context"
	"fmt"
	"net/http"

	grpchealth "github.com/bufbuild/connect-grpchealth-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
	"github.com/thanos-io/thanos/pkg/discovery/dns"
	"github.com/weaveworks/common/server"
	"github.com/weaveworks/common/user"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/grafana/fire/pkg/agent"
	"github.com/grafana/fire/pkg/distributor"
	"github.com/grafana/fire/pkg/firedb"
	agentv1 "github.com/grafana/fire/pkg/gen/agent/v1"
	"github.com/grafana/fire/pkg/gen/agent/v1/agentv1connect"
	"github.com/grafana/fire/pkg/gen/ingester/v1/ingesterv1connect"
	"github.com/grafana/fire/pkg/gen/push/v1/pushv1connect"
	"github.com/grafana/fire/pkg/gen/querier/v1/querierv1connect"
	"github.com/grafana/fire/pkg/ingester"
	objstoreclient "github.com/grafana/fire/pkg/objstore/client"
	"github.com/grafana/fire/pkg/openapiv2"
	"github.com/grafana/fire/pkg/querier"
	"github.com/grafana/fire/pkg/util"
)

// The various modules that make up Fire.
const (
	All          string = "all"
	Agent        string = "agent"
	Distributor  string = "distributor"
	Server       string = "server"
	Ring         string = "ring"
	Ingester     string = "ingester"
	MemberlistKV string = "memberlist-kv"
	Querier      string = "querier"
	GRPCGateway  string = "grpc-gateway"
	FireDB       string = "firedb"
	Storage      string = "storage"

	// RuntimeConfig            string = "runtime-config"
	// Overrides                string = "overrides"
	// OverridesExporter        string = "overrides-exporter"
	// TenantConfigs            string = "tenant-configs"
	// IngesterQuerier          string = "ingester-querier"
	// QueryFrontend            string = "query-frontend"
	// QueryFrontendTripperware string = "query-frontend-tripperware"
	// RulerStorage             string = "ruler-storage"
	// Ruler                    string = "ruler"
	// TableManager             string = "table-manager"
	// Compactor                string = "compactor"
	// IndexGateway             string = "index-gateway"
	// IndexGatewayRing         string = "index-gateway-ring"
	// QueryScheduler           string = "query-scheduler"
	// UsageReport              string = "usage-report"
)

func (f *Fire) initQuerier() (services.Service, error) {
	q, err := querier.New(f.Cfg.Querier, f.ring, nil, f.logger)
	if err != nil {
		return nil, err
	}
	// Those API are not meant to stay but allows us for testing through Grafana.
	f.Server.HTTP.Handle("/pyroscope/render", http.HandlerFunc(q.RenderHandler))
	f.Server.HTTP.Handle("/pyroscope/label-values", http.HandlerFunc(q.LabelValuesHandler))
	f.Server.HTTP.Handle("/prometheus/api/v1/query_range", http.HandlerFunc(q.PrometheusQueryRangeHandler))
	f.Server.HTTP.Handle("/prometheus/api/v1/query", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// just return empty instant query result to make grafana explore display something in "both" mode
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	querierv1connect.RegisterQuerierServiceHandler(f.Server.HTTP, q)

	return q, nil
}

func (f *Fire) getPusherClient() pushv1connect.PusherServiceClient {
	return f.pusherClient
}

func (f *Fire) initGRPCGateway() (services.Service, error) {
	f.grpcGatewayMux = grpcgw.NewServeMux()
	f.Server.HTTP.NewRoute().PathPrefix("/api").Handler(f.grpcGatewayMux)

	return nil, nil
}

func (f *Fire) initDistributor() (services.Service, error) {
	d, err := distributor.New(f.Cfg.Distributor, f.ring, nil, f.reg, f.logger)
	if err != nil {
		return nil, err
	}

	// initialise direct pusher, this overwrites the default HTTP client
	f.pusherClient = d

	pushv1connect.RegisterPusherServiceHandler(f.Server.HTTP, d)
	return d, nil
}

func (f *Fire) initAgent() (services.Service, error) {
	a, err := agent.New(&f.Cfg.AgentConfig, f.logger, f.getPusherClient)
	if err != nil {
		return nil, err
	}
	f.agent = a

	// register endpoint at grpc gateway
	if err := agentv1.RegisterAgentServiceHandlerServer(context.Background(), f.grpcGatewayMux, a); err != nil {
		return nil, err
	}

	agentv1connect.RegisterAgentServiceHandler(f.Server.HTTP, a.ConnectHandler())
	return a, nil
}

func (f *Fire) initMemberlistKV() (services.Service, error) {
	f.Cfg.MemberlistKV.MetricsRegisterer = f.reg
	f.Cfg.MemberlistKV.Codecs = []codec.Codec{
		ring.GetCodec(),
	}

	dnsProviderReg := prometheus.WrapRegistererWithPrefix(
		"fire_",
		prometheus.WrapRegistererWith(
			prometheus.Labels{"name": "memberlist"},
			f.reg,
		),
	)
	dnsProvider := dns.NewProvider(f.logger, dnsProviderReg, dns.GolangResolverType)

	f.MemberlistKV = memberlist.NewKVInitService(&f.Cfg.MemberlistKV, f.logger, dnsProvider, f.reg)

	f.Cfg.Ingester.LifecyclerConfig.RingConfig.KVStore.MemberlistKV = f.MemberlistKV.GetMemberlistKV

	return f.MemberlistKV, nil
}

func (f *Fire) initRing() (_ services.Service, err error) {
	f.ring, err = ring.New(f.Cfg.Ingester.LifecyclerConfig.RingConfig, "ingester", "ring", f.logger, prometheus.WrapRegistererWithPrefix("fire_", f.reg))
	if err != nil {
		return nil, err
	}
	f.Server.HTTP.Path("/ring").Methods("GET", "POST").Handler(f.ring)
	return f.ring, nil
}

func (f *Fire) initFireDB() (_ services.Service, err error) {
	f.fireDB, err = firedb.New(&f.Cfg.FireDB, f.logger, f.reg)
	if err != nil {
		return nil, err
	}

	return f.fireDB, nil
}

func (f *Fire) initStorage() (_ services.Service, err error) {
	if cfg := f.Cfg.Storage.BucketConfig; len(cfg) > 0 {
		b, err := objstoreclient.NewBucket(
			f.logger,
			[]byte(cfg),
			f.reg,
			"storage",
		)
		if err != nil {
			return nil, errors.Wrap(err, "unable to initialise bucket")
		}
		f.storageBucket = b
	}

	return nil, nil
}

func (f *Fire) initIngester() (_ services.Service, err error) {
	f.Cfg.Ingester.LifecyclerConfig.ListenPort = f.Cfg.Server.HTTPListenPort

	ingester, err := ingester.New(f.Cfg.Ingester, f.logger, f.reg, f.fireDB, f.storageBucket)
	if err != nil {
		return nil, err
	}
	prefix, handler := grpchealth.NewHandler(grpchealth.NewStaticChecker(ingesterv1connect.IngesterServiceName))
	f.Server.HTTP.NewRoute().PathPrefix(prefix).Handler(handler)
	ingesterv1connect.RegisterIngesterServiceHandler(f.Server.HTTP, ingester)
	return ingester, nil
}

func (f *Fire) initServer() (services.Service, error) {
	prometheus.MustRegister(version.NewCollector("fire"))
	DisableSignalHandling(&f.Cfg.Server)
	f.Cfg.Server.Registerer = prometheus.WrapRegistererWithPrefix("fire_", f.reg)
	serv, err := server.New(f.Cfg.Server)
	if err != nil {
		return nil, err
	}

	f.Server = serv

	servicesToWaitFor := func() []services.Service {
		svs := []services.Service(nil)
		for m, s := range f.serviceMap {
			// Server should not wait for itself.
			if m != Server {
				svs = append(svs, s)
			}
		}
		return svs
	}

	s := NewServerService(f.Server, servicesToWaitFor, f.logger)
	// Best effort to propagate the org ID from the start.
	f.Server.HTTPServer.Handler = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !f.Cfg.AuthEnabled {
				// todo change to configurable tenant ID
				next.ServeHTTP(w, r.WithContext(user.InjectOrgID(r.Context(), "fake")))
				return
			}
			_, ctx, _ := user.ExtractOrgIDFromHTTPRequest(r)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}(f.Server.HTTPServer.Handler)
	// todo configure http2
	f.Server.HTTPServer.Handler = h2c.NewHandler(f.Server.HTTPServer.Handler, &http2.Server{})
	f.Server.HTTPServer.Handler = util.RecoveryHTTPMiddleware.Wrap(f.Server.HTTPServer.Handler)

	// expose openapiv2 definition
	openapiv2Handler, err := openapiv2.Handler()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize openapiv2 handler: %w", err)
	}
	f.Server.HTTP.Handle("/api/swagger.json", openapiv2Handler)

	return s, nil
}

// NewServerService constructs service from Server component.
// servicesToWaitFor is called when server is stopping, and should return all
// services that need to terminate before server actually stops.
// N.B.: this function is NOT Cortex specific, please let's keep it that way.
// Passed server should not react on signals. Early return from Run function is considered to be an error.
func NewServerService(serv *server.Server, servicesToWaitFor func() []services.Service, log log.Logger) services.Service {
	serverDone := make(chan error, 1)

	runFn := func(ctx context.Context) error {
		go func() {
			defer close(serverDone)
			serverDone <- serv.Run()
		}()

		select {
		case <-ctx.Done():
			return nil
		case err := <-serverDone:
			if err != nil {
				return err
			}
			return fmt.Errorf("server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		// wait until all modules are done, and then shutdown server.
		for _, s := range servicesToWaitFor() {
			_ = s.AwaitTerminated(context.Background())
		}

		// shutdown HTTP and gRPC servers (this also unblocks Run)
		serv.Shutdown()

		// if not closed yet, wait until server stops.
		<-serverDone
		level.Info(log).Log("msg", "server stopped")
		return nil
	}

	return services.NewBasicService(nil, runFn, stoppingFn)
}

// DisableSignalHandling puts a dummy signal handler
func DisableSignalHandling(config *server.Config) {
	config.SignalHandler = make(ignoreSignalHandler)
}

type ignoreSignalHandler chan struct{}

func (dh ignoreSignalHandler) Loop() {
	<-dh
}

func (dh ignoreSignalHandler) Stop() {
	close(dh)
}
