package distributor

import (
	"bytes"
	"context"
	"encoding/json"
	"expvar"
	"flag"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/limiter"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/distributor/aggregator"
	"github.com/grafana/pyroscope/pkg/distributor/ingestlimits"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/distributor/sampling"
	"github.com/grafana/pyroscope/pkg/distributor/writepath"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/pprofsplit"
	"github.com/grafana/pyroscope/pkg/model/relabel"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/slices"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/usagestats"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/validation"
)

type PushClient interface {
	Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
}

const (
	// distributorRingKey is the key under which we store the distributors ring in the KVStore.
	distributorRingKey = "distributor"

	// ringAutoForgetUnhealthyPeriods is how many consecutive timeout periods an unhealthy instance
	// in the ring will be automatically removed after.
	ringAutoForgetUnhealthyPeriods = 10

	ProfileName = "__name__"
)

// Config for a Distributor.
type Config struct {
	PushTimeout time.Duration
	PoolConfig  clientpool.PoolConfig `yaml:"pool_config,omitempty"`

	// Distributors ring
	DistributorRing util.CommonRingConfig `yaml:"ring"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet, logger log.Logger) {
	cfg.PoolConfig.RegisterFlagsWithPrefix("distributor", fs)
	fs.DurationVar(&cfg.PushTimeout, "distributor.push.timeout", 5*time.Second, "Timeout when pushing data to ingester.")
	cfg.DistributorRing.RegisterFlags("distributor.ring.", "collectors/", "distributors", fs, logger)
}

// Distributor coordinates replicates and distribution of log streams.
type Distributor struct {
	services.Service
	logger log.Logger

	cfg           Config
	limits        Limits
	ingestersRing ring.ReadRing
	pool          *ring_client.Pool

	// The global rate limiter requires a distributors ring to count
	// the number of healthy instances
	distributorsLifecycler *ring.BasicLifecycler
	distributorsRing       *ring.Ring
	healthyInstancesCount  *atomic.Uint32
	ingestionRateLimiter   *limiter.RateLimiter
	aggregator             *aggregator.MultiTenantAggregator[*pprof.ProfileMerge]
	asyncRequests          sync.WaitGroup
	ingestionLimitsSampler *ingestlimits.Sampler
	usageGroupEvaluator    *validation.UsageGroupEvaluator

	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	// Metrics and stats.
	metrics                 *metrics
	rfStats                 *expvar.Int
	bytesReceivedStats      *usagestats.Statistics
	bytesReceivedTotalStats *usagestats.Counter
	profileReceivedStats    *usagestats.MultiCounter
	profileSizeStats        *usagestats.MultiStatistics

	router        *writepath.Router
	segmentWriter writepath.SegmentWriterClient
}

type Limits interface {
	IngestionRateBytes(tenantID string) float64
	IngestionBurstSizeBytes(tenantID string) int
	IngestionLimit(tenantID string) *ingestlimits.Config
	DistributorSampling(tenantID string) *sampling.Config
	IngestionTenantShardSize(tenantID string) int
	MaxLabelNameLength(tenantID string) int
	MaxLabelValueLength(tenantID string) int
	MaxLabelNamesPerSeries(tenantID string) int
	MaxProfileSizeBytes(tenantID string) int
	MaxProfileStacktraceSamples(tenantID string) int
	MaxProfileStacktraceSampleLabels(tenantID string) int
	MaxProfileStacktraceDepth(tenantID string) int
	MaxProfileSymbolValueLength(tenantID string) int
	MaxSessionsPerSeries(tenantID string) int
	EnforceLabelsOrder(tenantID string) bool
	IngestionRelabelingRules(tenantID string) []*relabel.Config
	DistributorUsageGroups(tenantID string) *validation.UsageGroupConfig
	validation.ProfileValidationLimits
	aggregator.Limits
	writepath.Overrides
}

func New(
	config Config,
	ingesterRing ring.ReadRing,
	ingesterClientFactory ring_client.PoolFactory,
	limits Limits,
	reg prometheus.Registerer,
	logger log.Logger,
	segmentWriter writepath.SegmentWriterClient,
	ingesterClientsOptions ...connect.ClientOption,
) (*Distributor, error) {
	ingesterClientsOptions = append(
		connectapi.DefaultClientOptions(),
		ingesterClientsOptions...,
	)

	clients := promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Namespace: "pyroscope",
		Name:      "distributor_ingester_clients",
		Help:      "The current number of ingester clients.",
	})
	d := &Distributor{
		cfg:                     config,
		logger:                  logger,
		ingestersRing:           ingesterRing,
		pool:                    clientpool.NewIngesterPool(config.PoolConfig, ingesterRing, ingesterClientFactory, clients, logger, ingesterClientsOptions...),
		segmentWriter:           segmentWriter,
		metrics:                 newMetrics(reg),
		healthyInstancesCount:   atomic.NewUint32(0),
		aggregator:              aggregator.NewMultiTenantAggregator[*pprof.ProfileMerge](limits, reg),
		limits:                  limits,
		rfStats:                 usagestats.NewInt("distributor_replication_factor"),
		bytesReceivedStats:      usagestats.NewStatistics("distributor_bytes_received"),
		bytesReceivedTotalStats: usagestats.NewCounter("distributor_bytes_received_total"),
		profileReceivedStats:    usagestats.NewMultiCounter("distributor_profiles_received", "lang"),
		profileSizeStats:        usagestats.NewMultiStatistics("distributor_profile_sizes", "lang"),
	}

	ingesterRoute := writepath.IngesterFunc(d.sendRequestsToIngester)
	segmentWriterRoute := writepath.IngesterFunc(d.sendRequestsToSegmentWriter)
	d.router = writepath.NewRouter(
		logger, reg, limits,
		ingesterRoute,
		segmentWriterRoute,
	)

	var err error
	subservices := []services.Service(nil)
	subservices = append(subservices, d.pool)

	distributorsRing, distributorsLifecycler, err := newRingAndLifecycler(config.DistributorRing, d.healthyInstancesCount, logger, reg)
	if err != nil {
		return nil, err
	}

	d.ingestionLimitsSampler = ingestlimits.NewSampler(distributorsRing)
	d.usageGroupEvaluator = validation.NewUsageGroupEvaluator(logger)

	subservices = append(subservices, distributorsLifecycler, distributorsRing, d.aggregator, d.ingestionLimitsSampler)

	d.ingestionRateLimiter = limiter.NewRateLimiter(newGlobalRateStrategy(newIngestionRateStrategy(limits), d), 10*time.Second)
	d.distributorsLifecycler = distributorsLifecycler
	d.distributorsRing = distributorsRing

	d.subservices, err = services.NewManager(subservices...)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	d.subservicesWatcher = services.NewFailureWatcher()
	d.subservicesWatcher.WatchManager(d.subservices)

	d.Service = services.NewBasicService(d.starting, d.running, d.stopping)
	d.rfStats.Set(int64(ingesterRing.ReplicationFactor()))
	d.metrics.replicationFactor.Set(float64(ingesterRing.ReplicationFactor()))
	return d, nil
}

func (d *Distributor) starting(ctx context.Context) error {
	return services.StartManagerAndAwaitHealthy(ctx, d.subservices)
}

func (d *Distributor) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-d.subservicesWatcher.Chan():
		return errors.Wrap(err, "distributor subservice failed")
	}
}

func (d *Distributor) stopping(_ error) error {
	d.asyncRequests.Wait()
	return services.StopManagerAndAwaitStopped(context.Background(), d.subservices)
}

func (d *Distributor) Push(ctx context.Context, grpcReq *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	req := &distributormodel.PushRequest{
		Series: make([]*distributormodel.ProfileSeries, 0, len(grpcReq.Msg.Series)),
	}

	for _, grpcSeries := range grpcReq.Msg.Series {
		series := &distributormodel.ProfileSeries{
			Labels:  grpcSeries.Labels,
			Samples: make([]*distributormodel.ProfileSample, 0, len(grpcSeries.Samples)),
		}
		for _, grpcSample := range grpcSeries.Samples {
			profile, err := pprof.RawFromBytes(grpcSample.RawProfile)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}
			sample := &distributormodel.ProfileSample{
				Profile:    profile,
				RawProfile: grpcSample.RawProfile,
				ID:         grpcSample.ID,
			}
			req.RawProfileSize += len(grpcSample.RawProfile)
			series.Samples = append(series.Samples, sample)
		}
		req.Series = append(req.Series, series)
	}
	resp, err := d.PushParsed(ctx, req)
	if err != nil && validation.ReasonOf(err) != validation.Unknown {
		if sp := opentracing.SpanFromContext(ctx); sp != nil {
			ext.LogError(sp, err)
		}
		level.Debug(util.LoggerWithContext(ctx, d.logger)).Log("msg", "failed to validate profile", "err", err)
		return resp, err
	}
	return resp, err
}

func (d *Distributor) GetProfileLanguage(series *distributormodel.ProfileSeries) string {
	if series.Language != "" {
		return series.Language
	}
	if len(series.Samples) == 0 {
		return "unknown"
	}
	lang := series.GetLanguage()
	if lang == "" {
		lang = pprof.GetLanguage(series.Samples[0].Profile)
	}
	series.Language = lang
	return series.Language
}

func (d *Distributor) PushParsed(ctx context.Context, req *distributormodel.PushRequest) (resp *connect.Response[pushv1.PushResponse], err error) {
	now := model.Now()
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	logger := log.With(d.logger, "tenant", tenantID)

	req.TenantID = tenantID
	for _, series := range req.Series {
		serviceName := phlaremodel.Labels(series.Labels).Get(phlaremodel.LabelNameServiceName)
		if serviceName == "" {
			series.Labels = append(series.Labels, &typesv1.LabelPair{Name: phlaremodel.LabelNameServiceName, Value: phlaremodel.AttrServiceNameFallback})
		}
		sort.Sort(phlaremodel.Labels(series.Labels))
	}

	haveRawPprof := req.RawProfileType == distributormodel.RawProfileTypePPROF
	d.bytesReceivedTotalStats.Inc(int64(req.RawProfileSize))
	d.bytesReceivedStats.Record(float64(req.RawProfileSize))
	if !haveRawPprof {
		// if a single profile contains multiple profile types/names (e.g. jfr) then there is no such thing as
		// compressed size per profile type as all profile types are compressed once together. So we can not count
		// compressed bytes per profile type. Instead we count compressed bytes per profile.
		profName := req.RawProfileType // use "jfr" as profile name
		d.metrics.receivedCompressedBytes.WithLabelValues(string(profName), tenantID).Observe(float64(req.RawProfileSize))
	}

	d.calculateRequestSize(req)

	// We don't support externally provided profile annotations right now.
	// They are unfortunately part of the Push API so we explicitly clear them here.
	req.ClearAnnotations()
	if err := d.checkIngestLimit(req); err != nil {
		level.Debug(logger).Log("msg", "rejecting push request due to global ingest limit", "tenant", tenantID)
		validation.DiscardedProfiles.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalBytesUncompressed))
		return nil, err
	}

	if err := d.rateLimit(tenantID, req); err != nil {
		return nil, err
	}

	usageGroups := d.limits.DistributorUsageGroups(tenantID)

	for _, series := range req.Series {
		profName := phlaremodel.Labels(series.Labels).Get(ProfileName)

		groups := d.usageGroupEvaluator.GetMatch(tenantID, usageGroups, series.Labels)
		if err := d.checkUsageGroupsIngestLimit(req, groups.Names()); err != nil {
			level.Debug(logger).Log("msg", "rejecting push request due to usage group ingest limit", "tenant", tenantID)
			validation.DiscardedProfiles.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalProfiles))
			validation.DiscardedBytes.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalBytesUncompressed))
			groups.CountDiscardedBytes(string(validation.IngestLimitReached), req.TotalBytesUncompressed)
			return nil, err
		}

		if sample := d.shouldSample(tenantID, groups.Names()); !sample {
			level.Debug(logger).Log("msg", "skipping push request due to sampling", "tenant", tenantID)
			validation.DiscardedProfiles.WithLabelValues(string(validation.SkippedBySamplingRules), tenantID).Add(float64(req.TotalProfiles))
			validation.DiscardedBytes.WithLabelValues(string(validation.SkippedBySamplingRules), tenantID).Add(float64(req.TotalBytesUncompressed))
			groups.CountDiscardedBytes(string(validation.SkippedBySamplingRules), req.TotalBytesUncompressed)
			return connect.NewResponse(&pushv1.PushResponse{}), nil
		}

		profLanguage := d.GetProfileLanguage(series)

		for _, raw := range series.Samples {
			usagestats.NewCounter(fmt.Sprintf("distributor_profile_type_%s_received", profName)).Inc(1)
			d.profileReceivedStats.Inc(1, profLanguage)
			if haveRawPprof {
				d.metrics.receivedCompressedBytes.WithLabelValues(profName, tenantID).Observe(float64(len(raw.RawProfile)))
			}
			p := raw.Profile
			decompressedSize := p.SizeVT()
			d.metrics.receivedDecompressedBytes.WithLabelValues(profName, tenantID).Observe(float64(decompressedSize))
			d.metrics.receivedSamples.WithLabelValues(profName, tenantID).Observe(float64(len(p.Sample)))
			d.profileSizeStats.Record(float64(decompressedSize), profLanguage)
			groups.CountReceivedBytes(profName, int64(decompressedSize))

			if err = validation.ValidateProfile(d.limits, tenantID, p.Profile, decompressedSize, series.Labels, now); err != nil {
				_ = level.Debug(logger).Log("msg", "invalid profile", "err", err)
				reason := string(validation.ReasonOf(err))
				validation.DiscardedProfiles.WithLabelValues(reason, tenantID).Add(float64(req.TotalProfiles))
				validation.DiscardedBytes.WithLabelValues(reason, tenantID).Add(float64(req.TotalBytesUncompressed))
				groups.CountDiscardedBytes(reason, req.TotalBytesUncompressed)
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}

			symbolsSize, samplesSize := profileSizeBytes(p.Profile)
			d.metrics.receivedSamplesBytes.WithLabelValues(profName, tenantID).Observe(float64(samplesSize))
			d.metrics.receivedSymbolsBytes.WithLabelValues(profName, tenantID).Observe(float64(symbolsSize))
		}
	}

	if req.TotalProfiles == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no profiles received"))
	}

	// Normalisation is quite an expensive operation,
	// therefore it should be done after the rate limit check.
	for _, series := range req.Series {
		for _, sample := range series.Samples {
			if series.Language == "go" {
				sample.Profile.Profile = pprof.FixGoProfile(sample.Profile.Profile)
			}
			sample.Profile.Normalize()
		}
	}

	removeEmptySeries(req)
	if len(req.Series) == 0 {
		// TODO(kolesnikovae):
		//   Normalization may cause all profiles and series to be empty.
		//   We should report it as an error and account for discarded data.
		//   The check should be done after ValidateProfile and normalization.
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	if err := injectMappingVersions(req.Series); err != nil {
		_ = level.Warn(logger).Log("msg", "failed to inject mapping versions", "err", err)
	}

	// Reduce cardinality of the session_id label.
	maxSessionsPerSeries := d.limits.MaxSessionsPerSeries(req.TenantID)
	for _, series := range req.Series {
		series.Labels = d.limitMaxSessionsPerSeries(maxSessionsPerSeries, series.Labels)
	}

	aggregated, err := d.aggregate(ctx, req)
	if err != nil {
		return nil, err
	}
	if aggregated {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	// Write path router directs the request to the ingester or segment
	// writer, or both, depending on the configuration.
	// The router uses sendRequestsToSegmentWriter and sendRequestsToIngester
	// functions to send the request to the appropriate service; these are
	// called independently, and may be called concurrently: the request is
	// cloned in this case – the callee may modify the request safely.
	if err = d.router.Send(ctx, req); err != nil {
		return nil, err
	}

	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

// If aggregation is configured for the tenant, we try to determine
// whether the profile is eligible for aggregation based on the series
// profile rate, and handle it asynchronously, if this is the case.
//
// NOTE(kolesnikovae): aggregated profiles are handled on best-effort
// basis (at-most-once delivery semantics): any error occurred will
// not be returned to the client, and it must not retry sending.
//
// Aggregation is only meant to be used for cases, when clients do not
// form individual series (e.g., server-less workload), and typically
// are ephemeral in its nature, and therefore retrying is not possible
// or desirable, as it prolongs life-time duration of the clients.
func (d *Distributor) aggregate(ctx context.Context, req *distributormodel.PushRequest) (bool, error) {
	a, ok := d.aggregator.AggregatorForTenant(req.TenantID)
	if !ok {
		// Aggregation is not configured for the tenant.
		return false, nil
	}

	// Actually all series profiles can be merged before aggregation.
	// However, it's not expected that a series has more than one profile.
	if len(req.Series) != 1 {
		return false, nil
	}
	series := req.Series[0]
	if len(series.Samples) != 1 {
		return false, nil
	}

	// First, we drop __session_id__ label to increase probability
	// of aggregation, which is handled done per series.
	profile := series.Samples[0].Profile.Profile
	labels := phlaremodel.Labels(series.Labels)
	if _, hasSessionID := labels.GetLabel(phlaremodel.LabelNameSessionID); hasSessionID {
		labels = labels.Clone().Delete(phlaremodel.LabelNameSessionID)
	}
	r, ok, err := a.Aggregate(labels.Hash(), profile.TimeNanos, mergeProfile(profile))
	if err != nil {
		return false, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if !ok {
		// Aggregation is not needed.
		return false, nil
	}
	handler := r.Handler()
	if handler == nil {
		// Aggregation is handled in another goroutine.
		return true, nil
	}

	// Aggregation is needed, and we own the result handler.
	// Note that the labels include the source series labels with
	// session ID: this is required to ensure fair load distribution.
	d.asyncRequests.Add(1)
	labels = phlaremodel.Labels(req.Series[0].Labels).Clone()
	annotations := req.Series[0].Annotations
	go func() {
		defer d.asyncRequests.Done()
		sendErr := util.RecoverPanic(func() error {
			localCtx, cancel := context.WithTimeout(context.Background(), d.cfg.PushTimeout)
			defer cancel()
			localCtx = tenant.InjectTenantID(localCtx, req.TenantID)
			if sp := opentracing.SpanFromContext(ctx); sp != nil {
				localCtx = opentracing.ContextWithSpan(localCtx, sp)
			}
			// Obtain the aggregated profile.
			p, handleErr := handler()
			if handleErr != nil {
				return handleErr
			}
			aggregated := &distributormodel.PushRequest{
				TenantID: req.TenantID,
				Series: []*distributormodel.ProfileSeries{{
					Labels:      labels,
					Samples:     []*distributormodel.ProfileSample{{Profile: pprof.RawFromProto(p.Profile())}},
					Annotations: annotations,
				}},
			}
			return d.router.Send(localCtx, aggregated)
		})()
		if sendErr != nil {
			_ = level.Error(d.logger).Log("msg", "failed to handle aggregation", "tenant", req.TenantID, "err", err)
		}
	}()

	return true, nil
}

// visitSampleSeriesForIngester creates a profile per unique label set in pprof labels.
func visitSampleSeriesForIngester(profile *profilev1.Profile, labels []*typesv1.LabelPair, rules []*relabel.Config, visitor *sampleSeriesVisitor) error {
	return pprofsplit.VisitSampleSeries(profile, labels, rules, visitor)
}

func (d *Distributor) sendRequestsToIngester(ctx context.Context, req *distributormodel.PushRequest) (resp *connect.Response[pushv1.PushResponse], err error) {
	if err = d.visitSampleSeries(req, visitSampleSeriesForIngester); err != nil {
		return nil, err
	}
	if len(req.Series) == 0 {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	enforceLabelOrder := d.limits.EnforceLabelsOrder(req.TenantID)
	keys := make([]uint32, len(req.Series))
	for i, s := range req.Series {
		if enforceLabelOrder {
			s.Labels = phlaremodel.Labels(s.Labels).InsertSorted(phlaremodel.LabelNameOrder, phlaremodel.LabelOrderEnforced)
		}
		keys[i] = TokenFor(req.TenantID, phlaremodel.LabelPairsString(s.Labels))
	}

	profiles := make([]*profileTracker, 0, len(req.Series))
	for _, series := range req.Series {
		for _, raw := range series.Samples {
			p := raw.Profile
			// zip the data back into the buffer
			bw := bytes.NewBuffer(raw.RawProfile[:0])
			if _, err = p.WriteTo(bw); err != nil {
				return nil, err
			}
			raw.ID = uuid.NewString()
			raw.RawProfile = bw.Bytes()
		}
		profiles = append(profiles, &profileTracker{profile: series})
	}

	const maxExpectedReplicationSet = 5 // typical replication factor 3 plus one for inactive plus one for luck
	var descs [maxExpectedReplicationSet]ring.InstanceDesc

	samplesByIngester := map[string][]*profileTracker{}
	ingesterDescs := map[string]ring.InstanceDesc{}
	for i, key := range keys {
		// Get a subring if tenant has shuffle shard size configured.
		subRing := d.ingestersRing.ShuffleShard(req.TenantID, d.limits.IngestionTenantShardSize(req.TenantID))

		replicationSet, err := subRing.Get(key, ring.Write, descs[:0], nil, nil)
		if err != nil {
			return nil, err
		}
		profiles[i].minSuccess = len(replicationSet.Instances) - replicationSet.MaxErrors
		profiles[i].maxFailures = replicationSet.MaxErrors
		for _, ingester := range replicationSet.Instances {
			samplesByIngester[ingester.Addr] = append(samplesByIngester[ingester.Addr], profiles[i])
			ingesterDescs[ingester.Addr] = ingester
		}
	}
	tracker := pushTracker{
		done: make(chan struct{}, 1), // buffer avoids blocking if caller terminates - sendProfiles() only sends once on each
		err:  make(chan error, 1),
	}
	tracker.samplesPending.Store(int32(len(profiles)))
	for ingester, samples := range samplesByIngester {
		go func(ingester ring.InstanceDesc, samples []*profileTracker) {
			// Use a background context to make sure all ingesters get samples even if we return early
			localCtx, cancel := context.WithTimeout(context.Background(), d.cfg.PushTimeout)
			defer cancel()
			localCtx = tenant.InjectTenantID(localCtx, req.TenantID)
			if sp := opentracing.SpanFromContext(ctx); sp != nil {
				localCtx = opentracing.ContextWithSpan(localCtx, sp)
			}
			d.sendProfiles(localCtx, ingester, samples, &tracker)
		}(ingesterDescs[ingester], samples)
	}
	select {
	case err = <-tracker.err:
		return nil, err
	case <-tracker.done:
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// visitSampleSeriesForSegmentWriter creates a profile per service.
// Labels that are shared by all pprof samples are used as series labels.
// Unique sample labels (not present in series labels) are preserved:
// pprof split takes place in segment-writers.
func visitSampleSeriesForSegmentWriter(profile *profilev1.Profile, labels []*typesv1.LabelPair, rules []*relabel.Config, visitor *sampleSeriesVisitor) error {
	return pprofsplit.VisitSampleSeriesBy(profile, labels, rules, visitor, phlaremodel.LabelNameServiceName)
}

func (d *Distributor) sendRequestsToSegmentWriter(ctx context.Context, req *distributormodel.PushRequest) (*connect.Response[pushv1.PushResponse], error) {
	// NOTE(kolesnikovae): if we return early, e.g., due to a validation error,
	//   or if there are no series, the write path router has already seen the
	//   request, and could have already accounted for the size, latency, etc.
	if err := d.visitSampleSeries(req, visitSampleSeriesForSegmentWriter); err != nil {
		return nil, err
	}
	if len(req.Series) == 0 {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	// TODO(kolesnikovae): Add profiles per request histogram.
	// In most cases, we only have a single profile. We should avoid
	// batching multiple profiles into a single request: overhead of handling
	// multiple profiles in a single request is substantial: we need to
	// allocate memory for all profiles at once, and wait for multiple requests
	// routed to different shards to complete is generally a bad idea because
	// it's hard to reason about latencies, retries, and error handling.
	config := d.limits.WritePathOverrides(req.TenantID)
	requests := make([]*segmentwriterv1.PushRequest, 0, len(req.Series)*2)
	for _, s := range req.Series {
		for _, p := range s.Samples {
			buf, err := pprof.Marshal(p.Profile.Profile, config.Compression == writepath.CompressionGzip)
			if err != nil {
				panic(fmt.Sprintf("failed to marshal profile: %v", err))
			}
			// Ideally, the ID should identify the whole request, and be
			// deterministic (e.g, based on the request hash). In practice,
			// the API allows batches, which makes it difficult to handle.
			profileID := uuid.New()
			requests = append(requests, &segmentwriterv1.PushRequest{
				TenantId:    req.TenantID,
				Labels:      s.Labels,
				Profile:     buf,
				ProfileId:   profileID[:],
				Annotations: s.Annotations,
			})
		}
	}

	if len(requests) == 1 {
		if _, err := d.segmentWriter.Push(ctx, requests[0]); err != nil {
			return nil, err
		}
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	// Fallback. We should minimize probability of this branch.
	g, ctx := errgroup.WithContext(ctx)
	for _, r := range requests {
		r := r
		g.Go(func() error {
			_, pushErr := d.segmentWriter.Push(ctx, r)
			return pushErr
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

// profileSizeBytes returns the size of symbols and samples in bytes.
func profileSizeBytes(p *profilev1.Profile) (symbols, samples int64) {
	fullSize := p.SizeVT()
	// remove samples
	samplesSlice := p.Sample
	p.Sample = nil

	symbols = int64(p.SizeVT())
	samples = int64(fullSize) - symbols

	// count labels in samples
	samplesLabels := 0
	for _, s := range samplesSlice {
		for _, l := range s.Label {
			samplesLabels += len(p.StringTable[l.Key]) + len(p.StringTable[l.Str]) + len(p.StringTable[l.NumUnit])
		}
	}
	symbols -= int64(samplesLabels)
	samples += int64(samplesLabels)

	// restore samples
	p.Sample = samplesSlice
	return
}

func mergeProfile(profile *profilev1.Profile) aggregator.AggregateFn[*pprof.ProfileMerge] {
	return func(m *pprof.ProfileMerge) (*pprof.ProfileMerge, error) {
		if m == nil {
			m = new(pprof.ProfileMerge)
		}
		if err := m.Merge(profile); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return m, nil
	}
}

func (d *Distributor) sendProfiles(ctx context.Context, ingester ring.InstanceDesc, profileTrackers []*profileTracker, pushTracker *pushTracker) {
	err := d.sendProfilesErr(ctx, ingester, profileTrackers)
	// If we succeed, decrement each sample's pending count by one.  If we reach
	// the required number of successful puts on this sample, then decrement the
	// number of pending samples by one.  If we successfully push all samples to
	// min success ingesters, wake up the waiting rpc so it can return early.
	// Similarly, track the number of errors, and if it exceeds maxFailures
	// shortcut the waiting rpc.
	//
	// The use of atomic increments here guarantees only a single sendSamples
	// goroutine will write to either channel.
	for i := range profileTrackers {
		if err != nil {
			if profileTrackers[i].failed.Inc() <= int32(profileTrackers[i].maxFailures) {
				continue
			}
			if pushTracker.samplesFailed.Inc() == 1 {
				pushTracker.err <- err
			}
		} else {
			if profileTrackers[i].succeeded.Inc() != int32(profileTrackers[i].minSuccess) {
				continue
			}
			if pushTracker.samplesPending.Dec() == 0 {
				pushTracker.done <- struct{}{}
			}
		}
	}
}

func (d *Distributor) sendProfilesErr(ctx context.Context, ingester ring.InstanceDesc, profileTrackers []*profileTracker) error {
	c, err := d.pool.GetClientFor(ingester.Addr)
	if err != nil {
		return err
	}

	req := connect.NewRequest(&pushv1.PushRequest{
		Series: make([]*pushv1.RawProfileSeries, 0, len(profileTrackers)),
	})

	for _, p := range profileTrackers {
		series := &pushv1.RawProfileSeries{
			Labels:      p.profile.Labels,
			Samples:     make([]*pushv1.RawSample, 0, len(p.profile.Samples)),
			Annotations: p.profile.Annotations,
		}
		for _, sample := range p.profile.Samples {
			series.Samples = append(series.Samples, &pushv1.RawSample{
				RawProfile: sample.RawProfile,
				ID:         sample.ID,
			})
		}
		req.Msg.Series = append(req.Msg.Series, series)
	}

	_, err = c.(PushClient).Push(ctx, req)
	return err
}

func (d *Distributor) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if d.distributorsRing != nil {
		d.distributorsRing.ServeHTTP(w, req)
	} else {
		ringNotEnabledPage := `
			<!DOCTYPE html>
			<html>
				<head>
					<meta charset="UTF-8">
					<title>Distributor Status</title>
				</head>
				<body>
					<h1>Distributor Status</h1>
					<p>Distributor is not running with global limits enabled</p>
				</body>
			</html>`
		util.WriteHTMLResponse(w, ringNotEnabledPage)
	}
}

// HealthyInstancesCount implements the ReadLifecycler interface
//
// We use a ring lifecycler delegate to count the number of members of the
// ring. The count is then used to enforce rate limiting correctly for each
// distributor. $EFFECTIVE_RATE_LIMIT = $GLOBAL_RATE_LIMIT / $NUM_INSTANCES
func (d *Distributor) HealthyInstancesCount() int {
	return int(d.healthyInstancesCount.Load())
}

func (d *Distributor) limitMaxSessionsPerSeries(maxSessionsPerSeries int, labels phlaremodel.Labels) phlaremodel.Labels {
	if maxSessionsPerSeries == 0 {
		return labels.Delete(phlaremodel.LabelNameSessionID)
	}
	sessionIDLabel, ok := labels.GetLabel(phlaremodel.LabelNameSessionID)
	if !ok {
		return labels
	}
	sessionID, err := phlaremodel.ParseSessionID(sessionIDLabel.Value)
	if err != nil {
		_ = level.Debug(d.logger).Log("msg", "invalid session_id", "err", err)
		return labels.Delete(phlaremodel.LabelNameSessionID)
	}
	sessionIDLabel.Value = phlaremodel.SessionID(int(sessionID) % maxSessionsPerSeries).String()
	return labels
}

func (d *Distributor) rateLimit(tenantID string, req *distributormodel.PushRequest) error {
	if !d.ingestionRateLimiter.AllowN(time.Now(), tenantID, int(req.TotalBytesUncompressed)) {
		validation.DiscardedProfiles.WithLabelValues(string(validation.RateLimited), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.RateLimited), tenantID).Add(float64(req.TotalBytesUncompressed))
		return connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("push rate limit (%s) exceeded while adding %s", humanize.IBytes(uint64(d.limits.IngestionRateBytes(tenantID))), humanize.IBytes(uint64(req.TotalBytesUncompressed))),
		)
	}
	return nil
}

func (d *Distributor) calculateRequestSize(req *distributormodel.PushRequest) {
	for _, series := range req.Series {
		// include the labels in the size calculation
		for _, lbs := range series.Labels {
			req.TotalBytesUncompressed += int64(len(lbs.Name))
			req.TotalBytesUncompressed += int64(len(lbs.Value))
		}
		for _, raw := range series.Samples {
			req.TotalProfiles += 1
			req.TotalBytesUncompressed += int64(raw.Profile.SizeVT())
		}
	}
}

func (d *Distributor) checkIngestLimit(req *distributormodel.PushRequest) error {
	l := d.limits.IngestionLimit(req.TenantID)
	if l == nil {
		return nil
	}

	if l.LimitReached {
		// we want to allow a very small portion of the traffic after reaching the limit
		if d.ingestionLimitsSampler.AllowRequest(req.TenantID, l.Sampling) {
			if err := req.MarkThrottledTenant(l); err != nil {
				return err
			}
			return nil
		}
		limitResetTime := time.Unix(l.LimitResetTime, 0).UTC().Format(time.RFC3339)
		return connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("limit of %s/%s reached, next reset at %s", humanize.IBytes(uint64(l.PeriodLimitMb*1024*1024)), l.PeriodType, limitResetTime))
	}

	return nil
}

func (d *Distributor) checkUsageGroupsIngestLimit(req *distributormodel.PushRequest, groupsInRequest []validation.UsageGroupMatchName) error {
	l := d.limits.IngestionLimit(req.TenantID)
	if l == nil || len(l.UsageGroups) == 0 {
		return nil
	}

	for _, group := range groupsInRequest {
		limit, ok := l.UsageGroups[group.ResolvedName]
		if !ok {
			limit, ok = l.UsageGroups[group.ConfiguredName]
		}
		if !ok || !limit.LimitReached {
			continue
		}
		if d.ingestionLimitsSampler.AllowRequest(req.TenantID, l.Sampling) {
			if err := req.MarkThrottledUsageGroup(l, group.ResolvedName); err != nil {
				return err
			}
			return nil
		}
		limitResetTime := time.Unix(l.LimitResetTime, 0).UTC().Format(time.RFC3339)
		return connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("limit of %s/%s reached for usage group %s, next reset at %s", humanize.IBytes(uint64(limit.PeriodLimitMb*1024*1024)), l.PeriodType, group, limitResetTime))
	}

	return nil
}

func (d *Distributor) shouldSample(tenantID string, groupsInRequest []validation.UsageGroupMatchName) bool {
	l := d.limits.DistributorSampling(tenantID)
	if l == nil {
		return true
	}

	// Determine the minimum probability among all matching usage groups.
	minProb := 1.0
	matched := false
	for _, group := range groupsInRequest {
		if probCfg, ok := l.UsageGroups[group.ResolvedName]; ok {
			matched = true
			if probCfg.Probability < minProb {
				minProb = probCfg.Probability
			}
			continue
		}
		if probCfg, ok := l.UsageGroups[group.ConfiguredName]; ok {
			matched = true
			if probCfg.Probability < minProb {
				minProb = probCfg.Probability
			}
		}
	}

	// If no sampling rules matched, accept the request.
	if !matched {
		return true
	}

	// Sample once using the minimum probability.
	return rand.Float64() <= minProb
}

type profileTracker struct {
	profile     *distributormodel.ProfileSeries
	minSuccess  int
	maxFailures int
	succeeded   atomic.Int32
	failed      atomic.Int32
}

type pushTracker struct {
	samplesPending atomic.Int32
	samplesFailed  atomic.Int32
	done           chan struct{}
	err            chan error
}

// TokenFor generates a token used for finding ingesters from ring
func TokenFor(tenantID, labels string) uint32 {
	h := fnv.New32()
	_, _ = h.Write([]byte(tenantID))
	_, _ = h.Write([]byte(labels))
	return h.Sum32()
}

// newRingAndLifecycler creates a new distributor ring and lifecycler with all required lifecycler delegates
func newRingAndLifecycler(cfg util.CommonRingConfig, instanceCount *atomic.Uint32, logger log.Logger, reg prometheus.Registerer) (*ring.Ring, *ring.BasicLifecycler, error) {
	reg = prometheus.WrapRegistererWithPrefix("pyroscope_", reg)
	kvStore, err := kv.NewClient(cfg.KVStore, ring.GetCodec(), kv.RegistererWithKVName(reg, "distributor-lifecycler"), logger)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize distributors' KV store")
	}

	lifecyclerCfg, err := toBasicLifecyclerConfig(cfg, logger)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to build distributors' lifecycler config")
	}

	var delegate ring.BasicLifecyclerDelegate
	delegate = ring.NewInstanceRegisterDelegate(ring.ACTIVE, lifecyclerCfg.NumTokens)
	delegate = newHealthyInstanceDelegate(instanceCount, cfg.HeartbeatTimeout, delegate)
	delegate = ring.NewLeaveOnStoppingDelegate(delegate, logger)
	delegate = ring.NewAutoForgetDelegate(ringAutoForgetUnhealthyPeriods*cfg.HeartbeatTimeout, delegate, logger)

	distributorsLifecycler, err := ring.NewBasicLifecycler(lifecyclerCfg, "distributor", distributorRingKey, kvStore, delegate, logger, reg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize distributors' lifecycler")
	}

	distributorsRing, err := ring.New(cfg.ToRingConfig(), "distributor", distributorRingKey, logger, reg)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize distributors' ring client")
	}

	return distributorsRing, distributorsLifecycler, nil
}

// injectMappingVersions extract from the labels the mapping version and inject it into the profile's main mapping. (mapping[0])
func injectMappingVersions(series []*distributormodel.ProfileSeries) error {
	for _, s := range series {
		version, ok := phlaremodel.ServiceVersionFromLabels(s.Labels)
		if !ok {
			continue
		}
		for _, sample := range s.Samples {
			for _, m := range sample.Profile.Mapping {
				version.BuildID = sample.Profile.StringTable[m.BuildId]
				versionString, err := json.Marshal(version)
				if err != nil {
					return err
				}
				sample.Profile.StringTable = append(sample.Profile.StringTable, string(versionString))
				m.BuildId = int64(len(sample.Profile.StringTable) - 1)
			}
		}
	}
	return nil
}

type visitFunc func(*profilev1.Profile, []*typesv1.LabelPair, []*relabel.Config, *sampleSeriesVisitor) error

func (d *Distributor) visitSampleSeries(req *distributormodel.PushRequest, visit visitFunc) error {
	relabelingRules := d.limits.IngestionRelabelingRules(req.TenantID)
	usageConfig := d.limits.DistributorUsageGroups(req.TenantID)
	var result []*distributormodel.ProfileSeries

	for _, series := range req.Series {
		usageGroups := d.usageGroupEvaluator.GetMatch(req.TenantID, usageConfig, series.Labels)
		for _, p := range series.Samples {
			visitor := &sampleSeriesVisitor{
				tenantID: req.TenantID,
				limits:   d.limits,
				profile:  p.Profile,
			}
			if err := visit(p.Profile.Profile, series.Labels, relabelingRules, visitor); err != nil {
				validation.DiscardedProfiles.WithLabelValues(string(validation.ReasonOf(err)), req.TenantID).Add(float64(req.TotalProfiles))
				validation.DiscardedBytes.WithLabelValues(string(validation.ReasonOf(err)), req.TenantID).Add(float64(req.TotalBytesUncompressed))
				usageGroups.CountDiscardedBytes(string(validation.ReasonOf(err)), req.TotalBytesUncompressed)
				return connect.NewError(connect.CodeInvalidArgument, err)
			}
			for _, s := range visitor.series {
				s.Annotations = series.Annotations
				s.Language = series.Language
				result = append(result, s)
			}
			req.DiscardedProfilesRelabeling += int64(visitor.discardedProfiles)
			req.DiscardedBytesRelabeling += int64(visitor.discardedBytes)
			if visitor.discardedBytes > 0 {
				usageGroups.CountDiscardedBytes(string(validation.DroppedByRelabelRules), int64(visitor.discardedBytes))
			}
		}
	}

	if req.DiscardedBytesRelabeling > 0 {
		validation.DiscardedBytes.WithLabelValues(string(validation.DroppedByRelabelRules), req.TenantID).Add(float64(req.DiscardedBytesRelabeling))
	}
	if req.DiscardedProfilesRelabeling > 0 {
		validation.DiscardedProfiles.WithLabelValues(string(validation.DroppedByRelabelRules), req.TenantID).Add(float64(req.DiscardedProfilesRelabeling))
	}

	req.Series = result
	removeEmptySeries(req)
	return nil
}

func removeEmptySeries(req *distributormodel.PushRequest) {
	for _, s := range req.Series {
		s.Samples = slices.RemoveInPlace(s.Samples, func(sample *distributormodel.ProfileSample, _ int) bool {
			return len(sample.Profile.Sample) == 0
		})
	}
	req.Series = slices.RemoveInPlace(req.Series, func(series *distributormodel.ProfileSeries, i int) bool {
		return len(series.Samples) == 0
	})
}

type sampleSeriesVisitor struct {
	tenantID string
	limits   Limits
	profile  *pprof.Profile
	exp      *pprof.SampleExporter
	series   []*distributormodel.ProfileSeries

	discardedBytes    int
	discardedProfiles int
}

func (v *sampleSeriesVisitor) ValidateLabels(labels phlaremodel.Labels) error {
	return validation.ValidateLabels(v.limits, v.tenantID, labels)
}

func (v *sampleSeriesVisitor) VisitProfile(labels phlaremodel.Labels) {
	v.series = append(v.series, &distributormodel.ProfileSeries{
		Samples: []*distributormodel.ProfileSample{{Profile: v.profile}},
		Labels:  labels,
	})
}

func (v *sampleSeriesVisitor) VisitSampleSeries(labels phlaremodel.Labels, samples []*profilev1.Sample) {
	if v.exp == nil {
		v.exp = pprof.NewSampleExporter(v.profile.Profile)
	}
	v.series = append(v.series, &distributormodel.ProfileSeries{
		Samples: []*distributormodel.ProfileSample{{Profile: exportSamples(v.exp, samples)}},
		Labels:  labels,
	})
}

func (v *sampleSeriesVisitor) Discarded(profiles, bytes int) {
	v.discardedProfiles += profiles
	v.discardedBytes += bytes
}

func exportSamples(e *pprof.SampleExporter, samples []*profilev1.Sample) *pprof.Profile {
	samplesCopy := make([]*profilev1.Sample, len(samples))
	copy(samplesCopy, samples)
	clear(samples)
	n := pprof.NewProfile()
	e.ExportSamples(n.Profile, samplesCopy)
	return n
}
