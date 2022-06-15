package ingester

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"

	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/polarsignals/arcticdb/query"
	"github.com/polarsignals/arcticdb/query/logicalplan"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"

	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/profilestore"
	"github.com/grafana/fire/pkg/util"
)

type Config struct {
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.LifecyclerConfig.RegisterFlags(f, util.Logger)
}

func (cfg *Config) Validate() error {
	return nil
}

type Ingester struct {
	services.Service

	cfg    Config
	logger log.Logger

	lifecycler        *ring.Lifecycler
	lifecyclerWatcher *services.FailureWatcher
	profileStore      *profilestore.ProfileStore
	engine            *query.LocalEngine
}

func New(cfg Config, logger log.Logger, reg prometheus.Registerer, profileStore *profilestore.ProfileStore) (*Ingester, error) {
	i := &Ingester{
		cfg:          cfg,
		logger:       logger,
		profileStore: profileStore,
		engine: query.NewEngine(
			memory.DefaultAllocator,
			profileStore.TableProvider(),
		),
	}

	var err error
	i.lifecycler, err = ring.NewLifecycler(
		cfg.LifecyclerConfig,
		i,
		"ingester",
		"ring",
		true,
		logger, prometheus.WrapRegistererWithPrefix("fire_", reg))
	if err != nil {
		return nil, err
	}

	i.lifecyclerWatcher = services.NewFailureWatcher()
	i.lifecyclerWatcher.WatchService(i.lifecycler)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *Ingester) starting(ctx context.Context) error {
	// pass new context to lifecycler, so that it doesn't stop automatically when Ingester's service context is done
	err := i.lifecycler.StartAsync(context.Background())
	if err != nil {
		return err
	}

	err = i.lifecycler.AwaitRunning(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (i *Ingester) running(ctx context.Context) error {
	var serviceError error
	select {
	// wait until service is asked to stop
	case <-ctx.Done():
	// stop
	case err := <-i.lifecyclerWatcher.Chan():
		serviceError = fmt.Errorf("lifecycler failed: %w", err)
	}
	return serviceError
}

func (i *Ingester) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	level.Debug(i.logger).Log("msg", "message received by ingester push", "request_headers: ", fmt.Sprintf("%+v", req.Header()))

	if err := i.profileStore.Ingest(ctx, req); err != nil {
		return nil, err
	}

	res := connect.NewResponse(&pushv1.PushResponse{})
	return res, nil
}

func (i *Ingester) RenderHandler(w http.ResponseWriter, req *http.Request) {
	q, err := parseQueryRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	query, err := parseQuery(q.query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	flame, err := i.selectMerge(req.Context(), query, q.start, q.end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(flame); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func matcherToBooleanExpression(matcher *labels.Matcher) (logicalplan.Expr, error) {
	ref := logicalplan.Col("labels." + matcher.Name)
	switch matcher.Type {
	case labels.MatchEqual:
		return ref.Eq(logicalplan.Literal(matcher.Value)), nil
	case labels.MatchNotEqual:
		return ref.NotEq(logicalplan.Literal(matcher.Value)), nil
	case labels.MatchRegexp:
		return ref.RegexMatch(matcher.Value), nil
	case labels.MatchNotRegexp:
		return ref.RegexNotMatch(matcher.Value), nil
	default:
		return nil, fmt.Errorf("unsupported matcher type %v", matcher.Type.String())
	}
}

func (i *Ingester) stopping(_ error) error {
	return services.StopAndAwaitTerminated(context.Background(), i.lifecycler)
}

func (i *Ingester) Flush() {
}

func (i *Ingester) TransferOut(ctx context.Context) error {
	return nil
}

// ReadinessHandler is used to indicate to k8s when the ingesters are ready for
// the addition removal of another ingester. Returns 204 when the ingester is
// ready, 500 otherwise.
func (i *Ingester) CheckReady(ctx context.Context) error {
	if s := i.State(); s != services.Running && s != services.Stopping {
		return fmt.Errorf("ingester not ready: %v", s)
	}
	return i.lifecycler.CheckReady(ctx)
}
