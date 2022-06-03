package fire

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
	"github.com/weaveworks/common/server"
	"github.com/weaveworks/common/user"

	"github.com/grafana/fire/pkg/agent"
)

// The various modules that make up Fire.
const (
	All         string = "all"
	Agent       string = "agent"
	Distributor string = "distributor"
	Server      string = "server"

	// Ring                     string = "ring"
	// RuntimeConfig            string = "runtime-config"
	// Overrides                string = "overrides"
	// OverridesExporter        string = "overrides-exporter"
	// TenantConfigs            string = "tenant-configs"
	// Ingester                 string = "ingester"
	// Querier                  string = "querier"
	// IngesterQuerier          string = "ingester-querier"
	// QueryFrontend            string = "query-frontend"
	// QueryFrontendTripperware string = "query-frontend-tripperware"
	// RulerStorage             string = "ruler-storage"
	// Ruler                    string = "ruler"
	// Store                    string = "store"
	// TableManager             string = "table-manager"
	// MemberlistKV             string = "memberlist-kv"
	// Compactor                string = "compactor"
	// IndexGateway             string = "index-gateway"
	// IndexGatewayRing         string = "index-gateway-ring"
	// QueryScheduler           string = "query-scheduler"
	// UsageReport              string = "usage-report"
)

func (f *Fire) initDistributor() (services.Service, error) {
	// todo
	return nil, nil
}

func (f *Fire) initAgent() (services.Service, error) {
	a, err := agent.New(&f.Cfg.AgentConfig, f.logger)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (f *Fire) initServer() (services.Service, error) {
	prometheus.MustRegister(version.NewCollector("loki"))
	DisableSignalHandling(&f.Cfg.Server)
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
				next.ServeHTTP(w, r.WithContext(user.InjectOrgID(r.Context(), "fake")))
				return
			}
			_, ctx, _ := user.ExtractOrgIDFromHTTPRequest(r)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}(f.Server.HTTPServer.Handler)

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
