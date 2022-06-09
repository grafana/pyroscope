package ingester

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/apache/arrow/go/v8/arrow/memory"
	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gogo/status"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/polarsignals/arcticdb/query"
	"github.com/polarsignals/arcticdb/query/logicalplan"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"google.golang.org/grpc/codes"

	"github.com/grafana/fire/assets"
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
}

func New(cfg Config, logger log.Logger, reg prometheus.Registerer, profileStore *profilestore.ProfileStore) (*Ingester, error) {
	i := &Ingester{
		cfg:          cfg,
		logger:       logger,
		profileStore: profileStore,
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
	eng := query.NewEngine(
		memory.DefaultAllocator,
		i.profileStore.TableProvider(),
	)

	filterExpr, err := parseQuery(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var ar arrow.Record
	err = eng.ScanTable("stacktraces").
		Filter(filterExpr).
		Aggregate(
			logicalplan.Sum(logicalplan.Col("value")),
			logicalplan.Col("stacktrace"),
		).
		Execute(req.Context(), func(r arrow.Record) error {
			r.Retain()
			ar = r
			return nil
		})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}
	defer ar.Release()

	flame, err := buildProfile(ar, i.profileStore.MetaStore())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "text/html")
	a, err := assets.Assets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := flamebearer.FlamebearerToStandaloneHTML(flame, a, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func buildProfile(ar arrow.Record, meta metastore.ProfileMetaStore) (*flamebearer.FlamebearerProfile, error) {
	type sample struct {
		stacktraceID []byte
		locationIDs  [][]byte
		value        int64
	}
	schema := ar.Schema()
	indices := schema.FieldIndices("stacktrace")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one stacktrace column, got %d", len(indices))
	}
	stacktraceColumn := ar.Column(indices[0]).(*array.Binary)

	indices = schema.FieldIndices("sum(value)")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one value column, got %d", len(indices))
	}
	valueColumn := ar.Column(indices[0]).(*array.Int64)

	rows := int(ar.NumRows())
	samples := make([]*sample, 0, rows)
	stacktraceUUIDs := make([][]byte, 0, rows)
	for i := 0; i < rows; i++ {
		stacktraceID := stacktraceColumn.Value(i)
		value := valueColumn.Value(i)

		stacktraceUUIDs = append(stacktraceUUIDs, stacktraceID)
		samples = append(samples, &sample{
			stacktraceID: stacktraceID,
			value:        value,
		})
	}

	stacktraceMap, err := meta.GetStacktraceByIDs(context.Background(), stacktraceUUIDs...)
	if err != nil {
		return nil, err
	}

	locationUUIDSeen := map[string]struct{}{}
	locationUUIDs := [][]byte{}
	for _, s := range stacktraceMap {
		for _, id := range s.GetLocationIds() {
			if _, seen := locationUUIDSeen[string(id)]; !seen {
				locationUUIDSeen[string(id)] = struct{}{}
				locationUUIDs = append(locationUUIDs, id)
			}
		}
	}

	locationsMap, err := metastore.GetLocationsByIDs(context.Background(), meta, locationUUIDs...)
	if err != nil {
		return nil, err
	}

	fmt.Println(locationsMap)

	for _, s := range samples {
		s.locationIDs = stacktraceMap[string(s.stacktraceID)].LocationIds
	}
	sort.Slice(samples, func(i, j int) bool {
		return len(samples[i].locationIDs) < len(samples[j].locationIDs)
	})

	return &flamebearer.FlamebearerProfile{
		FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
			Metadata: flamebearer.FlamebearerMetadataV1{
				Format: "single",
				Units:  "bytes",
				Name:   "inuse_memory",
			},
			Timeline: &flamebearer.FlamebearerTimelineV1{
				StartTime:     time.Now().Add(-1 * time.Hour).Unix(),
				DurationDelta: 3,
				Samples:       []uint64{1, 2, 3, 4, 5},
			},
			Flamebearer: flamebearer.FlamebearerV1{
				Names: []string{"total", "bar()"},
				Levels: [][]int{
					{0, 2036457, 0, 0},
					{0, 2036457, 2036457, 1},
				},
				NumTicks: 1,
				MaxSelf:  2036457,
			},
		},
	}, nil
}

// render/render?format=json&from=now-12h&until=now&query=pyroscope.server.cpu
func parseQuery(req *http.Request) (logicalplan.Expr, error) {
	queryParams := req.URL.Query()
	q := queryParams.Get("query")
	if q == "" {
		return nil, fmt.Errorf("query is required")
	}
	selectorExprs, err := queryToFilterExprs(q)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	start := model.TimeFromUnixNano(time.Now().Add(-1 * time.Hour).UnixNano())
	end := model.TimeFromUnixNano(time.Now().UnixNano())

	if from := queryParams.Get("from"); from != "" {
		from, err := parseRelativeTime(from)
		if err != nil {
			return nil, fmt.Errorf("failed to parse from: %w", err)
		}
		start = end.Add(-from)
	}

	return logicalplan.And(
		append(
			selectorExprs,
			logicalplan.Col("timestamp").GT(logicalplan.Literal(int64(start))),
			logicalplan.Col("timestamp").LT(logicalplan.Literal(int64(end))),
		)...,
	), nil
}

func parseRelativeTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "now-")
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d, nil
}

func queryToFilterExprs(query string) ([]logicalplan.Expr, error) {
	parsedSelector, err := parser.ParseMetricSelector(query)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse query")
	}

	sel := make([]*labels.Matcher, 0, len(parsedSelector))
	var nameLabel *labels.Matcher
	for _, matcher := range parsedSelector {
		if matcher.Name == labels.MetricName {
			nameLabel = matcher
		} else {
			sel = append(sel, matcher)
		}
	}
	if nameLabel == nil {
		return nil, status.Error(codes.InvalidArgument, "query must contain a profile-type selection")
	}

	parts := strings.Split(nameLabel.Value, ":")
	if len(parts) != 5 && len(parts) != 6 {
		return nil, status.Errorf(codes.InvalidArgument, "profile-type selection must be of the form <name>:<sample-type>:<sample-unit>:<period-type>:<period-unit>(:delta), got(%d): %q", len(parts), nameLabel.Value)
	}
	name, sampleType, sampleUnit, periodType, periodUnit, delta := parts[0], parts[1], parts[2], parts[3], parts[4], false
	if len(parts) == 6 && parts[5] == "delta" {
		delta = true
	}

	labelFilterExpressions, err := matchersToBooleanExpressions(sel)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to build query")
	}

	exprs := append([]logicalplan.Expr{
		logicalplan.Col("name").Eq(logicalplan.Literal(name)),
		logicalplan.Col("sample_type").Eq(logicalplan.Literal(sampleType)),
		logicalplan.Col("sample_unit").Eq(logicalplan.Literal(sampleUnit)),
		logicalplan.Col("period_type").Eq(logicalplan.Literal(periodType)),
		logicalplan.Col("period_unit").Eq(logicalplan.Literal(periodUnit)),
	}, labelFilterExpressions...)

	deltaPlan := logicalplan.Col("duration").Eq(logicalplan.Literal(0))
	if delta {
		deltaPlan = logicalplan.Col("duration").NotEq(logicalplan.Literal(0))
	}

	exprs = append(exprs, deltaPlan)

	return exprs, nil
}

func matchersToBooleanExpressions(matchers []*labels.Matcher) ([]logicalplan.Expr, error) {
	exprs := make([]logicalplan.Expr, 0, len(matchers))

	for _, matcher := range matchers {
		expr, err := matcherToBooleanExpression(matcher)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs, expr)
	}

	return exprs, nil
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
