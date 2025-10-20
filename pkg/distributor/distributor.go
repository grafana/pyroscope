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
	"go.uber.org/atomic"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/limiter"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
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
	"github.com/grafana/pyroscope/pkg/model/sampletype"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/usagestats"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/spanlogger"
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
	SampleTypeRelabelingRules(tenantID string) []*relabel.Config
	DistributorUsageGroups(tenantID string) *validation.UsageGroupConfig
	WritePathOverrides(tenantID string) writepath.Config
	validation.ProfileValidationLimits
	aggregator.Limits
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
	d.router = writepath.NewRouter(logger, reg, ingesterRoute, segmentWriterRoute)

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
		Series:         make([]*distributormodel.ProfileSeries, 0, len(grpcReq.Msg.Series)),
		RawProfileType: distributormodel.RawProfileTypePPROF,
	}
	allErrors := multierror.New()
	for _, grpcSeries := range grpcReq.Msg.Series {
		for _, grpcSample := range grpcSeries.Samples {
			profile, err := pprof.RawFromBytes(grpcSample.RawProfile)
			if err != nil {
				allErrors.Add(err)
				continue
			}
			series := &distributormodel.ProfileSeries{
				Labels:     grpcSeries.Labels,
				Profile:    profile,
				RawProfile: grpcSample.RawProfile,
				ID:         grpcSample.ID,
			}
			req.Series = append(req.Series, series)
		}
	}
	if err := d.PushBatch(ctx, req); err != nil {
		allErrors.Add(err)
	}
	err := allErrors.Err()
	if err != nil && validation.ReasonOf(err) != validation.Unknown {
		if sp := opentracing.SpanFromContext(ctx); sp != nil {
			ext.LogError(sp, err)
		}
		level.Debug(util.LoggerWithContext(ctx, d.logger)).Log("msg", "failed to validate profile", "err", err)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(new(pushv1.PushResponse)), err
}

func (d *Distributor) GetProfileLanguage(series *distributormodel.ProfileSeries) string {
	if series.Language != "" {
		return series.Language
	}
	lang := series.GetLanguage()
	if lang == "" {
		lang = pprof.GetLanguage(series.Profile)
	}
	series.Language = lang
	return series.Language
}

func (d *Distributor) PushBatch(ctx context.Context, req *distributormodel.PushRequest) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Distributor.PushBatch")
	defer sp.Finish()

	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	sp.SetTag("tenant_id", tenantID)

	if len(req.Series) == 0 {
		return noNewProfilesReceivedError()
	}

	d.bytesReceivedTotalStats.Inc(int64(req.ReceivedCompressedProfileSize))
	d.bytesReceivedStats.Record(float64(req.ReceivedCompressedProfileSize))
	if req.RawProfileType != distributormodel.RawProfileTypePPROF {
		// if a single profile contains multiple profile types/names (e.g. jfr) then there is no such thing as
		// compressed size per profile type as all profile types are compressed once together. So we can not count
		// compressed bytes per profile type. Instead we count compressed bytes per profile.
		profName := req.RawProfileType // use "jfr" as profile name
		d.metrics.receivedCompressedBytes.WithLabelValues(string(profName), tenantID).Observe(float64(req.ReceivedCompressedProfileSize))
	}

	res := multierror.New()
	errorsMutex := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for index, s := range req.Series {
		wg.Add(1)
		go func() {
			defer wg.Done()
			itErr := util.RecoverPanic(func() error {
				return d.pushSeries(ctx, s, req.RawProfileType, tenantID)
			})()

			if itErr != nil {
				itErr = fmt.Errorf("push series with index %d and id %s failed: %w", index, s.ID, itErr)
			}
			errorsMutex.Lock()
			res.Add(itErr)
			errorsMutex.Unlock()
		}()
	}
	wg.Wait()
	return res.Err()
}

type lazyUsageGroups func() []validation.UsageGroupMatchName

func (l lazyUsageGroups) String() string {
	groups := l()
	result := make([]string, len(groups))
	for pos := range groups {
		result[pos] = groups[pos].String()
	}
	return fmt.Sprintf("%v", result)
}

type pushLog struct {
	fields []any
	lvl    func(log.Logger) log.Logger
	msg    string
}

func newPushLog(capacity int) *pushLog {
	fields := make([]any, 2, (capacity+1)*2)
	fields[0] = "msg"
	return &pushLog{
		fields: fields,
	}
}

func (p *pushLog) addFields(fields ...any) {
	p.fields = append(p.fields, fields...)
}

func (p *pushLog) log(logger log.Logger, err error) {
	// determine log level
	if p.lvl == nil {
		if err != nil {
			p.lvl = level.Warn
		} else {
			p.lvl = level.Debug
		}
	}

	if err != nil {
		p.addFields("err", err)
	}

	// update message
	if p.msg == "" {
		if err != nil {
			p.msg = "profile rejected"
		} else {
			p.msg = "profile accepted"
		}
	}
	p.fields[1] = p.msg
	p.lvl(logger).Log(p.fields...)
}

func (d *Distributor) pushSeries(ctx context.Context, req *distributormodel.ProfileSeries, origin distributormodel.RawProfileType, tenantID string) (err error) {
	if req.Profile == nil {
		return noNewProfilesReceivedError()
	}
	now := model.Now()

	logger := spanlogger.FromContext(ctx, log.With(d.logger, "tenant", tenantID))
	finalLog := newPushLog(10)
	defer func() {
		finalLog.log(logger, err)
	}()

	req.TenantID = tenantID
	serviceName := phlaremodel.Labels(req.Labels).Get(phlaremodel.LabelNameServiceName)
	if serviceName == "" {
		req.Labels = append(req.Labels, &typesv1.LabelPair{Name: phlaremodel.LabelNameServiceName, Value: phlaremodel.AttrServiceNameFallback})
	} else {
		finalLog.addFields("service_name", serviceName)
	}
	sort.Sort(phlaremodel.Labels(req.Labels))

	if req.ID != "" {
		finalLog.addFields("profile_id", req.ID)
	}

	req.TotalProfiles = 1
	req.TotalBytesUncompressed = calculateRequestSize(req)
	d.metrics.observeProfileSize(tenantID, StageReceived, req.TotalBytesUncompressed)

	if err := d.checkIngestLimit(req); err != nil {
		finalLog.msg = "rejecting profile due to global ingest limit"
		finalLog.lvl = level.Debug
		validation.DiscardedProfiles.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalBytesUncompressed))
		return err
	}

	if err := d.rateLimit(tenantID, req); err != nil {
		return err
	}

	usageGroups := d.limits.DistributorUsageGroups(tenantID)

	profName := phlaremodel.Labels(req.Labels).Get(ProfileName)
	finalLog.addFields("profile_type", profName)

	groups := d.usageGroupEvaluator.GetMatch(tenantID, usageGroups, req.Labels)
	finalLog.addFields("matched_usage_groups", lazyUsageGroups(groups.Names))
	if err := d.checkUsageGroupsIngestLimit(req, groups.Names()); err != nil {
		finalLog.msg = "rejecting profile due to usage group ingest limit"
		finalLog.lvl = level.Debug
		validation.DiscardedProfiles.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.IngestLimitReached), tenantID).Add(float64(req.TotalBytesUncompressed))
		groups.CountDiscardedBytes(string(validation.IngestLimitReached), req.TotalBytesUncompressed)
		return err
	}

	willSample, samplingSource := d.shouldSample(tenantID, groups.Names())
	if !willSample {
		finalLog.addFields(
			"usage_group", samplingSource.UsageGroup,
			"probability", samplingSource.Probability,
		)
		finalLog.msg = "skipping profile due to sampling"
		validation.DiscardedProfiles.WithLabelValues(string(validation.SkippedBySamplingRules), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.SkippedBySamplingRules), tenantID).Add(float64(req.TotalBytesUncompressed))
		groups.CountDiscardedBytes(string(validation.SkippedBySamplingRules), req.TotalBytesUncompressed)
		return nil
	}
	if samplingSource != nil {
		if err := req.MarkSampledRequest(samplingSource); err != nil {
			return err
		}
	}

	profLanguage := d.GetProfileLanguage(req)
	if profLanguage != "" {
		finalLog.addFields("detected_language", profLanguage)
	}

	usagestats.NewCounter(fmt.Sprintf("distributor_profile_type_%s_received", profName)).Inc(1)
	d.profileReceivedStats.Inc(1, profLanguage)
	if origin == distributormodel.RawProfileTypePPROF {
		d.metrics.receivedCompressedBytes.WithLabelValues(profName, tenantID).Observe(float64(len(req.RawProfile)))
	}
	p := req.Profile
	decompressedSize := p.SizeVT()
	profTime := model.TimeFromUnixNano(p.TimeNanos).Time()
	finalLog.addFields(
		"profile_time", profTime,
		"ingestion_delay", now.Time().Sub(profTime),
		"decompressed_size", decompressedSize,
		"sample_count", len(p.Sample),
	)
	d.metrics.observeProfileSize(tenantID, StageSampled, int64(decompressedSize))                              //todo use req.TotalBytesUncompressed to include labels siz
	d.metrics.receivedDecompressedBytes.WithLabelValues(profName, tenantID).Observe(float64(decompressedSize)) // deprecated TODO remove
	d.metrics.receivedSamples.WithLabelValues(profName, tenantID).Observe(float64(len(p.Sample)))
	d.profileSizeStats.Record(float64(decompressedSize), profLanguage)
	groups.CountReceivedBytes(profName, int64(decompressedSize))

	validated, err := validation.ValidateProfile(d.limits, tenantID, p, decompressedSize, req.Labels, now)
	if err != nil {
		reason := string(validation.ReasonOf(err))
		finalLog.addFields("reason", reason)
		validation.DiscardedProfiles.WithLabelValues(reason, tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(reason, tenantID).Add(float64(req.TotalBytesUncompressed))
		groups.CountDiscardedBytes(reason, req.TotalBytesUncompressed)
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	symbolsSize, samplesSize := profileSizeBytes(p.Profile)
	d.metrics.receivedSamplesBytes.WithLabelValues(profName, tenantID).Observe(float64(samplesSize))
	d.metrics.receivedSymbolsBytes.WithLabelValues(profName, tenantID).Observe(float64(symbolsSize))

	// Normalisation is quite an expensive operation,
	// therefore it should be done after the rate limit check.
	if req.Language == "go" {
		sp, _ := opentracing.StartSpanFromContext(ctx, "pprof.FixGoProfile")
		req.Profile.Profile = pprof.FixGoProfile(req.Profile.Profile)
		sp.Finish()
	}
	{
		sp, _ := opentracing.StartSpanFromContext(ctx, "sampletype.Relabel")
		sampleTypeRules := d.limits.SampleTypeRelabelingRules(req.TenantID)
		sampletype.Relabel(validated, sampleTypeRules, req.Labels)
		sp.Finish()
	}
	{
		sp, _ := opentracing.StartSpanFromContext(ctx, "Profile.Normalize")
		req.Profile.Normalize()
		sp.Finish()
		d.metrics.observeProfileSize(tenantID, StageNormalized, calculateRequestSize(req))
	}

	if len(req.Profile.Sample) == 0 {
		// TODO(kolesnikovae):
		//   Normalization may cause all profiles and series to be empty.
		//   We should report it as an error and account for discarded data.
		//   The check should be done after ValidateProfile and normalization.
		return nil
	}

	if err := injectMappingVersions(req); err != nil {
		_ = level.Warn(logger).Log("msg", "failed to inject mapping versions", "err", err)
	}

	// Reduce cardinality of the session_id label.
	maxSessionsPerSeries := d.limits.MaxSessionsPerSeries(req.TenantID)
	req.Labels = d.limitMaxSessionsPerSeries(maxSessionsPerSeries, req.Labels)

	aggregated, err := d.aggregate(ctx, req)
	if err != nil {
		return err
	}
	if aggregated {
		return nil
	}

	// Write path router directs the request to the ingester or segment
	// writer, or both, depending on the configuration.
	// The router uses sendRequestsToSegmentWriter and sendRequestsToIngester
	// functions to send the request to the appropriate service; these are
	// called independently, and may be called concurrently: the request is
	// cloned in this case – the callee may modify the request safely.
	config := d.limits.WritePathOverrides(req.TenantID)
	return d.router.Send(ctx, req, config)
}

func noNewProfilesReceivedError() *connect.Error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no profiles received"))
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
func (d *Distributor) aggregate(ctx context.Context, req *distributormodel.ProfileSeries) (bool, error) {
	a, ok := d.aggregator.AggregatorForTenant(req.TenantID)
	if !ok {
		// Aggregation is not configured for the tenant.
		return false, nil
	}

	series := req

	// First, we drop __session_id__ label to increase probability
	// of aggregation, which is handled done per series.
	profile := series.Profile.Profile
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
	labels = phlaremodel.Labels(req.Labels).Clone()
	annotations := req.Annotations
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
			aggregated := &distributormodel.ProfileSeries{
				TenantID:    req.TenantID,
				Labels:      labels,
				Profile:     pprof.RawFromProto(p.Profile()),
				Annotations: annotations,
			}
			config := d.limits.WritePathOverrides(req.TenantID)
			return d.router.Send(localCtx, aggregated, config)
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

func (d *Distributor) sendRequestsToIngester(ctx context.Context, req *distributormodel.ProfileSeries) (resp *connect.Response[pushv1.PushResponse], err error) {
	sampleSeries, err := d.visitSampleSeries(req, visitSampleSeriesForIngester)
	if err != nil {
		return nil, err
	}
	if len(sampleSeries) == 0 {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	}

	enforceLabelOrder := d.limits.EnforceLabelsOrder(req.TenantID)
	keys := make([]uint32, len(sampleSeries))
	for i, s := range sampleSeries {
		if enforceLabelOrder {
			s.Labels = phlaremodel.Labels(s.Labels).InsertSorted(phlaremodel.LabelNameOrder, phlaremodel.LabelOrderEnforced)
		}
		keys[i] = TokenFor(req.TenantID, phlaremodel.LabelPairsString(s.Labels))
	}

	profiles := make([]*profileTracker, 0, len(sampleSeries))
	for _, series := range sampleSeries {
		p := series.Profile
		// zip the data back into the buffer
		bw := bytes.NewBuffer(series.RawProfile[:0])
		if _, err = p.WriteTo(bw); err != nil {
			return nil, err
		}
		series.ID = uuid.NewString()
		series.RawProfile = bw.Bytes()
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

func (d *Distributor) sendRequestsToSegmentWriter(ctx context.Context, req *distributormodel.ProfileSeries) (*connect.Response[pushv1.PushResponse], error) {
	// NOTE(kolesnikovae): if we return early, e.g., due to a validation error,
	//   or if there are no series, the write path router has already seen the
	//   request, and could have already accounted for the size, latency, etc.
	serviceSeries, err := d.visitSampleSeries(req, visitSampleSeriesForSegmentWriter)
	if err != nil {
		return nil, err
	}
	if len(serviceSeries) == 0 {
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
	requests := make([]*segmentwriterv1.PushRequest, 0, len(serviceSeries)*2)
	for _, s := range serviceSeries {
		buf, err := pprof.Marshal(s.Profile.Profile, config.Compression == writepath.CompressionGzip)
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
		if err := m.Merge(profile, true); err != nil {
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
			Labels: p.profile.Labels,
			Samples: []*pushv1.RawSample{{
				RawProfile: p.profile.RawProfile,
				ID:         p.profile.ID,
			}},
			Annotations: p.profile.Annotations,
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

func (d *Distributor) rateLimit(tenantID string, req *distributormodel.ProfileSeries) error {
	if !d.ingestionRateLimiter.AllowN(time.Now(), tenantID, int(req.TotalBytesUncompressed)) {
		validation.DiscardedProfiles.WithLabelValues(string(validation.RateLimited), tenantID).Add(float64(req.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.RateLimited), tenantID).Add(float64(req.TotalBytesUncompressed))
		return connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("push rate limit (%s) exceeded while adding %s", humanize.IBytes(uint64(d.limits.IngestionRateBytes(tenantID))), humanize.IBytes(uint64(req.TotalBytesUncompressed))),
		)
	}
	return nil
}

func calculateRequestSize(req *distributormodel.ProfileSeries) int64 {
	// include the labels in the size calculation
	bs := int64(0)
	for _, lbs := range req.Labels {
		bs += int64(len(lbs.Name))
		bs += int64(len(lbs.Value))
	}

	bs += int64(req.Profile.SizeVT())
	return bs
}

func (d *Distributor) checkIngestLimit(req *distributormodel.ProfileSeries) error {
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

func (d *Distributor) checkUsageGroupsIngestLimit(req *distributormodel.ProfileSeries, groupsInRequest []validation.UsageGroupMatchName) error {
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

// shouldSample returns true if the profile should be injected and optionally the usage group that was responsible for the decision.
func (d *Distributor) shouldSample(tenantID string, groupsInRequest []validation.UsageGroupMatchName) (bool, *sampling.Source) {
	l := d.limits.DistributorSampling(tenantID)
	if l == nil {
		return true, nil
	}

	samplingProbability := 1.0
	var match *validation.UsageGroupMatchName
	for _, group := range groupsInRequest {
		probabilityCfg, found := l.UsageGroups[group.ResolvedName]
		if !found {
			probabilityCfg, found = l.UsageGroups[group.ConfiguredName]
		}
		if !found {
			continue
		}
		// a less specific group loses to a more specific one
		if match != nil && match.IsMoreSpecificThan(&group) {
			continue
		}
		// lower probability wins; when tied, the more specific group wins
		if probabilityCfg.Probability <= samplingProbability {
			samplingProbability = probabilityCfg.Probability
			match = &group
		}
	}

	if match == nil {
		return true, nil
	}

	source := &sampling.Source{
		UsageGroup:  match.ResolvedName,
		Probability: samplingProbability,
	}

	return rand.Float64() <= samplingProbability, source
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
func injectMappingVersions(s *distributormodel.ProfileSeries) error {
	version, ok := phlaremodel.ServiceVersionFromLabels(s.Labels)
	if !ok {
		return nil
	}
	for _, m := range s.Profile.Mapping {
		version.BuildID = s.Profile.StringTable[m.BuildId]
		versionString, err := json.Marshal(version)
		if err != nil {
			return err
		}
		s.Profile.StringTable = append(s.Profile.StringTable, string(versionString))
		m.BuildId = int64(len(s.Profile.StringTable) - 1)
	}
	return nil
}

type visitFunc func(*profilev1.Profile, []*typesv1.LabelPair, []*relabel.Config, *sampleSeriesVisitor) error

func (d *Distributor) visitSampleSeries(s *distributormodel.ProfileSeries, visit visitFunc) ([]*distributormodel.ProfileSeries, error) {
	relabelingRules := d.limits.IngestionRelabelingRules(s.TenantID)
	usageConfig := d.limits.DistributorUsageGroups(s.TenantID)
	var result []*distributormodel.ProfileSeries
	usageGroups := d.usageGroupEvaluator.GetMatch(s.TenantID, usageConfig, s.Labels)
	visitor := &sampleSeriesVisitor{
		tenantID: s.TenantID,
		limits:   d.limits,
		profile:  s.Profile,
		logger:   d.logger,
	}
	if err := visit(s.Profile.Profile, s.Labels, relabelingRules, visitor); err != nil {
		validation.DiscardedProfiles.WithLabelValues(string(validation.ReasonOf(err)), s.TenantID).Add(float64(s.TotalProfiles))
		validation.DiscardedBytes.WithLabelValues(string(validation.ReasonOf(err)), s.TenantID).Add(float64(s.TotalBytesUncompressed))
		usageGroups.CountDiscardedBytes(string(validation.ReasonOf(err)), s.TotalBytesUncompressed)
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	for _, ss := range visitor.series {
		ss.Annotations = s.Annotations
		ss.Language = s.Language
		result = append(result, ss)
	}
	s.DiscardedProfilesRelabeling += int64(visitor.discardedProfiles)
	s.DiscardedBytesRelabeling += int64(visitor.discardedBytes)
	if visitor.discardedBytes > 0 {
		usageGroups.CountDiscardedBytes(string(validation.DroppedByRelabelRules), int64(visitor.discardedBytes))
	}

	if s.DiscardedBytesRelabeling > 0 {
		validation.DiscardedBytes.WithLabelValues(string(validation.DroppedByRelabelRules), s.TenantID).Add(float64(s.DiscardedBytesRelabeling))
	}
	if s.DiscardedProfilesRelabeling > 0 {
		validation.DiscardedProfiles.WithLabelValues(string(validation.DroppedByRelabelRules), s.TenantID).Add(float64(s.DiscardedProfilesRelabeling))
	}
	// todo should we do normalization after relabeling?
	return result, nil
}

type sampleSeriesVisitor struct {
	tenantID string
	limits   Limits
	profile  *pprof.Profile
	exp      *pprof.SampleExporter
	series   []*distributormodel.ProfileSeries
	logger   log.Logger

	discardedBytes    int
	discardedProfiles int
}

func (v *sampleSeriesVisitor) ValidateLabels(labels phlaremodel.Labels) error {
	return validation.ValidateLabels(v.limits, v.tenantID, labels, v.logger)
}

func (v *sampleSeriesVisitor) VisitProfile(labels phlaremodel.Labels) {
	v.series = append(v.series, &distributormodel.ProfileSeries{
		Profile: v.profile,
		Labels:  labels,
	})
}

func (v *sampleSeriesVisitor) VisitSampleSeries(labels phlaremodel.Labels, samples []*profilev1.Sample) {
	if v.exp == nil {
		v.exp = pprof.NewSampleExporter(v.profile.Profile)
	}
	v.series = append(v.series, &distributormodel.ProfileSeries{
		Profile: exportSamples(v.exp, samples),
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
