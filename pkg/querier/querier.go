package querier

import (
	"container/heap"
	"context"
	"flag"
	"sort"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/ingester/clientpool"
	"github.com/grafana/fire/pkg/util"
)

// todo: move to non global metrics.
var clients = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "fire",
	Name:      "querier_ingester_clients",
	Help:      "The current number of ingester clients.",
})

type Config struct {
	PoolConfig      clientpool.PoolConfig `yaml:"pool_config,omitempty"`
	ExtraQueryDelay time.Duration         `yaml:"extra_query_delay,omitempty"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.PoolConfig.RegisterFlagsWithPrefix("querier", fs)
	fs.DurationVar(&cfg.ExtraQueryDelay, "querier.extra-query-delay", 0, "Time to wait before sending more than the minimum successful query requests.")
}

type Querier struct {
	services.Service
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	cfg    Config
	logger log.Logger

	ingestersRing   ring.ReadRing
	pool            *ring_client.Pool
	ingesterQuerier *IngesterQuerier
}

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, logger log.Logger) (*Querier, error) {
	q := &Querier{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		pool:          clientpool.NewPool(cfg.PoolConfig, ingestersRing, factory, clients, logger),
	}
	var err error
	q.subservices, err = services.NewManager(q.pool)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	q.subservicesWatcher = services.NewFailureWatcher()
	q.subservicesWatcher.WatchManager(q.subservices)
	q.Service = services.NewBasicService(q.starting, q.running, q.stopping)
	q.ingesterQuerier = NewIngesterQuerier(q.pool, ingestersRing, cfg.ExtraQueryDelay)
	return q, nil
}

func (q *Querier) starting(ctx context.Context) error {
	return services.StartManagerAndAwaitHealthy(ctx, q.subservices)
}

func (q *Querier) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-q.subservicesWatcher.Chan():
		return errors.Wrap(err, "distributor subservice failed")
	}
}

func (q *Querier) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), q.subservices)
}

func (q *Querier) ProfileTypes(ctx context.Context) ([]string, error) {
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ic IngesterQueryClient) ([]string, error) {
		res, err := ic.ProfileTypes(ctx, connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}
	return uniqueSortedStrings(responses), nil
}

func (q *Querier) LabelValues(ctx context.Context, name string) ([]string, error) {
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelValues(ctx, connect.NewRequest(&ingestv1.LabelValuesRequest{
			Name: name,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}
	return uniqueSortedStrings(responses), nil
}

type profileResponsesHeap []responseFromIngesters[*ingestv1.SelectProfilesResponse]

// Implement sort.Interface
func (h profileResponsesHeap) Len() int      { return len(h) }
func (h profileResponsesHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h profileResponsesHeap) Less(i, j int) bool {
	return CompareProfile(h[i].response.Profiles[0], h[j].response.Profiles[0]) < 0
}

func CompareProfile(a, b *ingestv1.Profile) int64 {
	if a.Timestamp == b.Timestamp {
		return int64(util.CompareLabelPair(a.Labels, b.Labels))
	}
	return a.Timestamp - b.Timestamp
}

// Implement heap.Interface
func (h *profileResponsesHeap) Push(x interface{}) {
	*h = append(*h, x.(responseFromIngesters[*ingestv1.SelectProfilesResponse]))
}

func (h *profileResponsesHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// dedupeProfiles dedupes profiles responses by timestamp and labels.
// It expects profiles from each response to be sorted by timestamp and labels already.
func dedupeProfiles(responses []responseFromIngesters[*ingestv1.SelectProfilesResponse]) []*ingestv1.Profile {
	r := profileResponsesHeap(responses)
	h := heap.Interface(&r)
	var deduped []*ingestv1.Profile
	heap.Init(h)
	for h.Len() > 0 {
		top := heap.Pop(h).(responseFromIngesters[*ingestv1.SelectProfilesResponse])
		if len(deduped) == 0 || CompareProfile(top.response.Profiles[0], deduped[len(deduped)-1]) != 0 {
			deduped = append(deduped, top.response.Profiles[0])
		}
		top.response.Profiles = top.response.Profiles[1:]
		if len(top.response.Profiles) > 0 {
			heap.Push(h, top)
		}
	}
	return deduped
}

func (q *Querier) selectMerge(ctx context.Context, req *ingestv1.SelectProfilesRequest) (*flamebearer.FlamebearerProfile, error) {
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ic IngesterQueryClient) (*ingestv1.SelectProfilesResponse, error) {
		res, err := ic.SelectProfiles(ctx, connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	})
	if err != nil {
		return nil, err
	}
	// Start by ordering by timestamp then labels.
	for i := range responses {
		sort.Slice(responses[i].response.Profiles, func(j, k int) bool {
			if responses[i].response.Profiles[j].Timestamp == responses[i].response.Profiles[k].Timestamp {
				// we don't need the name label because we're querying only a single profile type at a time.
				return util.CompareLabelPair(responses[i].response.Profiles[j].Labels, responses[i].response.Profiles[k].Labels) < 0
			}
			return responses[i].response.Profiles[j].Timestamp < responses[i].response.Profiles[k].Timestamp
		})
	}

	var lastProfile *ingestv1.Profile
	profilePerIngester := make(map[string][]*ingestv1.Profile, len(responses))

	for i := range responses {
		if len(responses[i].response.Profiles) > 0 {
			continue
		}
		// if the profile is the same as the last one, we can skip it.
		if lastProfile != nil && lastProfile.Timestamp == responses[i].response.Profiles[0].Timestamp &&
			util.CompareLabelPair(lastProfile.Labels, responses[i].response.Profiles[0].Labels) == 0 {
			responses[i].response.Profiles = responses[i].response.Profiles[1:]
			continue
		}
		lastProfile = responses[i].response.Profiles[0]
		profilePerIngester[responses[i].addr] = append(profilePerIngester[responses[i].addr], lastProfile)
		responses[i].response.Profiles = responses[i].response.Profiles[1:]
		continue

	}

	// Merge stacktraces.

	return nil, nil
}

// func (q *Querier) selectMerge(ctx context.Context, req *ingesterv1.SelectProfilesRequest) (*flamebearer.FlamebearerProfile, error) {
// 	filterExpr, err := selectPlan(query, start, end)
// 	if err != nil {
// 		// todo 4xx
// 		return nil, err
// 	}

// 	var ar arrow.Record
// 	err = i.engine.ScanTable("stacktraces").
// 		Filter(filterExpr).
// 		Aggregate(
// 			logicalplan.Sum(logicalplan.Col("value")),
// 			logicalplan.Col("stacktrace"),
// 		).
// 		Execute(ctx, func(r arrow.Record) error {
// 			r.Retain()
// 			ar = r
// 			return nil
// 		})
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer ar.Release()
// 	flame, err := buildFlamebearer(ar, i.profileStore.MetaStore())
// 	if err != nil {
// 		return nil, err
// 	}
// 	unit := metadata.Units(query.sampleUnit)
// 	sampleRate := uint32(100)
// 	switch query.sampleType {
// 	case "inuse_objects", "alloc_objects", "goroutine", "samples":
// 		unit = metadata.ObjectsUnits
// 	case "cpu":
// 		unit = metadata.SamplesUnits
// 		sampleRate = uint32(100000000)

// 	}
// 	return &flamebearer.FlamebearerProfile{
// 		Version: 1,
// 		FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
// 			Flamebearer: *flame,
// 			Metadata: flamebearer.FlamebearerMetadataV1{
// 				Format:     "single",
// 				Units:      unit,
// 				Name:       query.sampleType,
// 				SampleRate: sampleRate,
// 			},
// 		},
// 	}, nil
// }

func uniqueSortedStrings(responses []responseFromIngesters[[]string]) []string {
	total := 0
	for _, r := range responses {
		total += len(r.response)
	}
	unique := make(map[string]struct{}, total)
	result := make([]string, 0, total)
	for _, r := range responses {
		for _, elem := range r.response {
			if _, ok := unique[elem]; !ok {
				unique[elem] = struct{}{}
				result = append(result, elem)
			}
		}
	}
	sort.Strings(result)
	return result
}
