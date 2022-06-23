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
	"github.com/grafana/fire/pkg/model"
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
	return model.CompareProfile(h[i].response.Profiles[0], h[j].response.Profiles[0]) < 0
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
// Returns the profile deduped per ingester address.
// todo: This function can be optimized by peeking instead of popping the heap.
func dedupeProfiles(responses []responseFromIngesters[*ingestv1.SelectProfilesResponse]) map[string][]*ingestv1.Profile {
	type tuple struct {
		ingester string
		profile  *ingestv1.Profile
		responseFromIngesters[*ingestv1.SelectProfilesResponse]
	}
	var (
		r                   = profileResponsesHeap(responses)
		h                   = heap.Interface(&r)
		deduped             []*ingestv1.Profile
		profilesPerIngester = make(map[string][]*ingestv1.Profile, len(responses))
		tuples              = make([]tuple, 0, len(responses))
	)

	heap.Init(h)
	for h.Len() > 0 || len(tuples) > 0 {
		if h.Len() > 0 {
			top := heap.Pop(h).(responseFromIngesters[*ingestv1.SelectProfilesResponse])

			if len(tuples) == 0 || model.CompareProfile(top.response.Profiles[0], tuples[len(tuples)-1].profile) == 0 {
				tuples = append(tuples, tuple{
					ingester:              top.addr,
					profile:               top.response.Profiles[0],
					responseFromIngesters: top,
				})
				top.response.Profiles = top.response.Profiles[1:]
				continue
			}
			// the current profile is different.
			heap.Push(h, top)
		}
		// if the heap is empty and we don't have tuples we're done.
		if len(tuples) == 0 {
			continue
		}
		// no duplicate found just a single profile.
		if len(tuples) == 1 {
			profilesPerIngester[tuples[0].addr] = append(profilesPerIngester[tuples[0].addr], tuples[0].profile)
			deduped = append(deduped, tuples[0].profile)
			if len(tuples[0].response.Profiles) > 0 {
				heap.Push(h, tuples[0].responseFromIngesters)
			}
			tuples = tuples[:0]
			continue
		}
		// we have a duplicate let's select a winner based on the ingester with the less profiles
		// this way we evenly distribute the profiles symbols API calls across the ingesters
		min := tuples[0]
		for _, t := range tuples {
			if len(profilesPerIngester[t.addr]) < len(profilesPerIngester[min.addr]) {
				min = t
			}
		}
		profilesPerIngester[min.addr] = append(profilesPerIngester[min.addr], min.profile)
		deduped = append(deduped, min.profile)
		if len(min.response.Profiles) > 0 {
			heap.Push(h, min.responseFromIngesters)
		}
		for _, t := range tuples {
			if t.addr != min.addr && len(t.response.Profiles) > 0 {
				heap.Push(h, t.responseFromIngesters)
				continue
			}
		}
		tuples = tuples[:0]

	}
	return profilesPerIngester
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

	_ = dedupeProfiles(responses)

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
