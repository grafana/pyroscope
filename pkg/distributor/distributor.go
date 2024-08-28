package distributor

import (
	"bytes"
	"context"
	"encoding/json"
	"expvar"
	"flag"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"sync"
	"time"
	"unsafe"

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

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/distributor/aggregator"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	writepath "github.com/grafana/pyroscope/pkg/distributor/write_path"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
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
	DistributorRing util.CommonRingConfig `yaml:"ring" doc:"hidden"`
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

	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	// Metrics and stats.
	metrics                 *metrics
	rfStats                 *expvar.Int
	bytesReceivedStats      *usagestats.Statistics
	bytesReceivedTotalStats *usagestats.Counter
	profileReceivedStats    *usagestats.MultiCounter
	profileSizeStats        *usagestats.MultiStatistics

	router *writepath.Router
}

type Limits interface {
	IngestionRateBytes(tenantID string) float64
	IngestionBurstSizeBytes(tenantID string) int
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
	segwriterClient writepath.SegmentWriterClient,
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

	ingesterClient := writepath.IngesterFunc(d.sendRequestsToIngester)
	d.router = writepath.NewRouter(
		logger, reg, limits,
		ingesterClient,
		segwriterClient,
	)

	var err error
	subservices := []services.Service(nil)
	subservices = append(subservices, d.pool)

	distributorsRing, distributorsLifecycler, err := newRingAndLifecycler(config.DistributorRing, d.healthyInstancesCount, logger, reg)
	if err != nil {
		return nil, err
	}

	subservices = append(subservices, distributorsLifecycler, distributorsRing, d.aggregator)

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
		lang = pprof.GetLanguage(series.Samples[0].Profile, d.logger)
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

	req.TenantID = tenantID
	for _, series := range req.Series {
		serviceName := phlaremodel.Labels(series.Labels).Get(phlaremodel.LabelNameServiceName)
		if serviceName == "" {
			series.Labels = append(series.Labels, &typesv1.LabelPair{Name: phlaremodel.LabelNameServiceName, Value: "unspecified"})
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

	if err := d.rateLimit(tenantID, req); err != nil {
		return nil, err
	}

	usageGroups := d.limits.DistributorUsageGroups(tenantID)

	for _, series := range req.Series {
		profName := phlaremodel.Labels(series.Labels).Get(ProfileName)
		groups := usageGroups.GetUsageGroups(tenantID, phlaremodel.Labels(series.Labels))
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
				_ = level.Debug(d.logger).Log("msg", "invalid profile", "err", err)
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

	if err := injectMappingVersions(req.Series); err != nil {
		_ = level.Warn(d.logger).Log("msg", "failed to inject mapping versions", "err", err)
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
					Labels:  labels,
					Samples: []*distributormodel.ProfileSample{{Profile: pprof.RawFromProto(p.Profile())}},
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

func (d *Distributor) sendRequestsToIngester(ctx context.Context, req *distributormodel.PushRequest) (resp *connect.Response[pushv1.PushResponse], err error) {
	// Next we split profiles by labels and apply relabeling rules.
	usageGroups := d.limits.DistributorUsageGroups(req.TenantID)
	profileSeries, bytesRelabelDropped, profilesRelabelDropped := extractSampleSeries(req, req.TenantID, usageGroups, d.limits.IngestionRelabelingRules(req.TenantID))
	validation.DiscardedBytes.WithLabelValues(string(validation.DroppedByRelabelRules), req.TenantID).Add(bytesRelabelDropped)
	validation.DiscardedProfiles.WithLabelValues(string(validation.DroppedByRelabelRules), req.TenantID).Add(profilesRelabelDropped)

	// Filter our series and profiles without samples.
	for _, series := range profileSeries {
		series.Samples = slices.RemoveInPlace(series.Samples, func(sample *distributormodel.ProfileSample, _ int) bool {
			return len(sample.Profile.Sample) == 0
		})
	}
	profileSeries = slices.RemoveInPlace(profileSeries, func(series *distributormodel.ProfileSeries, i int) bool {
		return len(series.Samples) == 0
	})
	if len(profileSeries) == 0 {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	// Validate the labels again and generate tokens for shuffle sharding.
	keys := make([]uint32, len(profileSeries))
	enforceLabelsOrder := d.limits.EnforceLabelsOrder(req.TenantID)
	for i, series := range profileSeries {
		if enforceLabelsOrder {
			series.Labels = phlaremodel.Labels(series.Labels).InsertSorted(phlaremodel.LabelNameOrder, phlaremodel.LabelOrderEnforced)
		}

		groups := usageGroups.GetUsageGroups(req.TenantID, series.Labels)

		if err = validation.ValidateLabels(d.limits, req.TenantID, series.Labels); err != nil {
			validation.DiscardedProfiles.WithLabelValues(string(validation.ReasonOf(err)), req.TenantID).Add(float64(req.TotalProfiles))
			validation.DiscardedBytes.WithLabelValues(string(validation.ReasonOf(err)), req.TenantID).Add(float64(req.TotalBytesUncompressed))
			groups.CountDiscardedBytes(string(validation.ReasonOf(err)), req.TotalBytesUncompressed)
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		keys[i] = TokenFor(req.TenantID, phlaremodel.LabelPairsString(series.Labels))
	}

	profiles := make([]*profileTracker, 0, len(profileSeries))
	for _, series := range profileSeries {
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

// sampleSize returns the size of a samples in bytes.
func sampleSize(stringTable []string, samplesSlice []*profilev1.Sample) int64 {
	var size int64
	for _, s := range samplesSlice {
		size += int64(s.SizeVT())
		for _, l := range s.Label {
			size += int64(len(stringTable[l.Key]) + len(stringTable[l.Str]) + len(stringTable[l.NumUnit]))
		}
	}
	return size
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
			Labels:  p.profile.Labels,
			Samples: make([]*pushv1.RawSample, 0, len(p.profile.Samples)),
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

type sampleKey struct {
	stacktrace string
	// note this is an index into the string table, rather than span ID
	spanIDIdx int64
}

func sampleKeyFromSample(stringTable []string, s *profilev1.Sample) sampleKey {
	var k sampleKey

	// populate spanID if present
	for _, l := range s.Label {
		if stringTable[int(l.Key)] == pprof.SpanIDLabelName {
			k.spanIDIdx = l.Str
		}
	}
	if len(s.LocationId) > 0 {
		k.stacktrace = unsafe.String(
			(*byte)(unsafe.Pointer(&s.LocationId[0])),
			len(s.LocationId)*8,
		)
	}
	return k
}

type lazyGroup struct {
	sampleGroup pprof.SampleGroup
	// The map is only initialized when the group is being modified. Key is the
	// string representation (unsafe) of the sample stack trace and its potential
	// span ID.
	sampleMap map[sampleKey]*profilev1.Sample
	labels    phlaremodel.Labels
}

func (g *lazyGroup) addSampleGroup(stringTable []string, sg pprof.SampleGroup) {
	if len(g.sampleGroup.Samples) == 0 {
		g.sampleGroup = sg
		return
	}

	// If the group is already initialized, we need to merge the samples.
	if g.sampleMap == nil {
		g.sampleMap = make(map[sampleKey]*profilev1.Sample)
		for _, s := range g.sampleGroup.Samples {
			g.sampleMap[sampleKeyFromSample(stringTable, s)] = s
		}
	}

	for _, s := range sg.Samples {
		k := sampleKeyFromSample(stringTable, s)
		if _, ok := g.sampleMap[k]; !ok {
			g.sampleGroup.Samples = append(g.sampleGroup.Samples, s)
			g.sampleMap[k] = s
		} else {
			// merge the samples
			for idx := range s.Value {
				g.sampleMap[k].Value[idx] += s.Value[idx]
			}
		}
	}
}

type groupsWithFingerprints struct {
	m     map[uint64][]lazyGroup
	order []uint64
}

func newGroupsWithFingerprints() *groupsWithFingerprints {
	return &groupsWithFingerprints{
		m: make(map[uint64][]lazyGroup),
	}
}

func (g *groupsWithFingerprints) add(stringTable []string, lbls phlaremodel.Labels, group pprof.SampleGroup) {
	fp := lbls.Hash()
	idxs, ok := g.m[fp]
	if ok {
		// fingerprint matches, check if the labels are the same
		for _, idx := range idxs {
			if phlaremodel.CompareLabelPairs(idx.labels, lbls) == 0 {
				// append samples to the group
				idx.addSampleGroup(stringTable, group)
				return
			}
		}
	} else {
		g.order = append(g.order, fp)
	}

	// add the labels to the list
	g.m[fp] = append(g.m[fp], lazyGroup{
		sampleGroup: group,
		labels:      lbls,
	})
}

func extractSampleSeries(req *distributormodel.PushRequest, tenantID string, usageGroups *validation.UsageGroupConfig, relabelRules []*relabel.Config) (result []*distributormodel.ProfileSeries, bytesRelabelDropped, profilesRelabelDropped float64) {
	var (
		lblbuilder = phlaremodel.NewLabelsBuilder(phlaremodel.EmptyLabels())
	)

	profileSeries := make([]*distributormodel.ProfileSeries, 0, len(req.Series))
	for _, series := range req.Series {
		s := &distributormodel.ProfileSeries{
			Labels:  series.Labels,
			Samples: make([]*distributormodel.ProfileSample, 0, len(series.Samples)),
		}
		usageGroups := usageGroups.GetUsageGroups(tenantID, series.Labels)

		for _, raw := range series.Samples {
			pprof.RenameLabel(raw.Profile.Profile, pprof.ProfileIDLabelName, pprof.SpanIDLabelName)
			groups := pprof.GroupSamplesWithoutLabels(raw.Profile.Profile, pprof.SpanIDLabelName)

			if len(groups) == 0 || (len(groups) == 1 && len(groups[0].Labels) == 0) {
				// No sample labels in the profile.

				// relabel the labels of the series
				lblbuilder.Reset(series.Labels)
				if len(relabelRules) > 0 {
					keep := relabel.ProcessBuilder(lblbuilder, relabelRules...)
					if !keep {
						bytesRelabelDropped += float64(raw.Profile.SizeVT())
						profilesRelabelDropped++ // in this case we dropped a whole profile
						usageGroups.CountDiscardedBytes(string(validation.DroppedByRelabelRules), int64(raw.Profile.SizeVT()))
						continue
					}
				}

				// Copy over the labels from the builder
				s.Labels = lblbuilder.Labels()

				// We do not modify the request.
				s.Samples = append(s.Samples, raw)

				continue
			}

			// iterate through groups relabel them and find relevant overlapping labelsets
			groupsKept := newGroupsWithFingerprints()
			for _, group := range groups {
				lblbuilder.Reset(series.Labels)
				addSampleLabelsToLabelsBuilder(lblbuilder, raw.Profile.Profile, group.Labels)
				if len(relabelRules) > 0 {
					keep := relabel.ProcessBuilder(lblbuilder, relabelRules...)
					if !keep {
						droppedBytes := sampleSize(raw.Profile.Profile.StringTable, group.Samples)
						bytesRelabelDropped += float64(droppedBytes)
						usageGroups.CountDiscardedBytes(string(validation.DroppedByRelabelRules), droppedBytes)
						continue
					}
				}

				// add the group to the list
				groupsKept.add(raw.Profile.StringTable, lblbuilder.Labels(), group)
			}

			if len(groupsKept.m) == 0 {
				// no groups kept, count the whole profile as dropped
				profilesRelabelDropped++
				continue
			}

			e := pprof.NewSampleExporter(raw.Profile.Profile)
			for _, idx := range groupsKept.order {
				for _, group := range groupsKept.m[idx] {
					// exportSamples creates a new profile with the samples provided.
					// The samples are obtained via GroupSamples call, which means
					// the underlying capacity is referenced by the source profile.
					// Therefore, the slice has to be copied and samples zeroed to
					// avoid ownership issues.
					profile := exportSamples(e, group.sampleGroup.Samples)
					profileSeries = append(profileSeries, &distributormodel.ProfileSeries{
						Labels:  group.labels,
						Samples: []*distributormodel.ProfileSample{{Profile: profile}},
					})
				}
			}
		}
		if len(s.Samples) > 0 {
			profileSeries = append(profileSeries, s)
		}
	}
	return profileSeries, bytesRelabelDropped, profilesRelabelDropped
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
	// rate limit the request
	if !d.ingestionRateLimiter.AllowN(time.Now(), tenantID, int(req.TotalBytesUncompressed)) {
		validation.DiscardedProfiles.WithLabelValues(string(validation.RateLimited), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.RateLimited), tenantID).Add(float64(req.TotalBytesUncompressed))
		return connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("push rate limit (%s) exceeded while adding %s", humanize.IBytes(uint64(d.limits.IngestionRateBytes(tenantID))), humanize.IBytes(uint64(req.TotalBytesUncompressed))),
		)
	}
	return nil
}

// addSampleLabelsToLabelsBuilder: adds sample label that don't exists yet on the profile builder. So the existing labels take precedence.
func addSampleLabelsToLabelsBuilder(b *phlaremodel.LabelsBuilder, p *profilev1.Profile, pl []*profilev1.Label) {
	var name string
	for _, l := range pl {
		name = p.StringTable[l.Key]
		if l.Str <= 0 {
			// skip if label value is not a string
			continue
		}
		if b.Get(name) != "" {
			// do nothing if label name already exists
			continue
		}
		b.Set(name, p.StringTable[l.Str])
	}
}

func exportSamples(e *pprof.SampleExporter, samples []*profilev1.Sample) *pprof.Profile {
	samplesCopy := make([]*profilev1.Sample, len(samples))
	copy(samplesCopy, samples)
	slices.Clear(samples)
	n := pprof.NewProfile()
	e.ExportSamples(n.Profile, samplesCopy)
	return n
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
