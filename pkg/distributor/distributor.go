package distributor

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/parca-dev/parca/pkg/scrape"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/atomic"

	commonv1 "github.com/grafana/phlare/pkg/gen/common/v1"
	pushv1 "github.com/grafana/phlare/pkg/gen/push/v1"
	"github.com/grafana/phlare/pkg/ingester/clientpool"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/pprof"
	"github.com/grafana/phlare/pkg/tenant"
	"github.com/grafana/phlare/pkg/usagestats"
)

type PushClient interface {
	Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
}

// todo: move to non global metrics.
var (
	clients = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "phlare",
		Name:      "distributor_ingester_clients",
		Help:      "The current number of ingester clients.",
	})
	rfStats                 = usagestats.NewInt("distributor_replication_factor")
	bytesReceivedStats      = usagestats.NewStatistics("distributor_bytes_received")
	bytesReceivedTotalStats = usagestats.NewCounter("distributor_bytes_received_total")
	profileReceivedStats    = usagestats.NewCounter("distributor_profiles_received")
)

// Config for a Distributor.
type Config struct {
	PushTimeout time.Duration
	PoolConfig  clientpool.PoolConfig `yaml:"pool_config,omitempty"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.PoolConfig.RegisterFlagsWithPrefix("distributor", fs)
	fs.DurationVar(&cfg.PushTimeout, "distributor.push.timeout", 5*time.Second, "Timeout when pushing data to ingester.")
}

// Distributor coordinates replicates and distribution of log streams.
type Distributor struct {
	services.Service
	logger log.Logger

	cfg           Config
	ingestersRing ring.ReadRing
	pool          *ring_client.Pool

	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	metrics *metrics
}

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, reg prometheus.Registerer, logger log.Logger, clientsOptions ...connect.ClientOption) (*Distributor, error) {
	d := &Distributor{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		pool:          clientpool.NewPool(cfg.PoolConfig, ingestersRing, factory, clients, logger, clientsOptions...),
		metrics:       newMetrics(reg),
	}
	var err error
	d.subservices, err = services.NewManager(d.pool)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	d.subservicesWatcher = services.NewFailureWatcher()
	d.subservicesWatcher.WatchManager(d.subservices)
	d.Service = services.NewBasicService(d.starting, d.running, d.stopping)
	rfStats.Set(int64(ingestersRing.ReplicationFactor()))
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
	return services.StopManagerAndAwaitStopped(context.Background(), d.subservices)
}

func (d *Distributor) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var (
		keys     = make([]uint32, 0, len(req.Msg.Series))
		profiles = make([]*profileTracker, 0, len(req.Msg.Series))
	)

	for _, series := range req.Msg.Series {
		keys = append(keys, TokenFor(tenantID, labelsString(series.Labels)))
		profName := phlaremodel.Labels(series.Labels).Get(scrape.ProfileName)
		for _, raw := range series.Samples {
			usagestats.NewCounter(fmt.Sprintf("distributor_profile_type_%s_received", profName)).Inc(1)
			profileReceivedStats.Inc(1)
			bytesReceivedTotalStats.Inc(int64(len(raw.RawProfile)))
			bytesReceivedStats.Record(float64(len(raw.RawProfile)))
			d.metrics.receivedCompressedBytes.WithLabelValues(profName).Observe(float64(len(raw.RawProfile)))
			p, err := pprof.RawFromBytes(raw.RawProfile)
			if err != nil {
				return nil, err
			}
			d.metrics.receivedDecompressedBytes.WithLabelValues(profName).Observe(float64(p.SizeBytes()))
			d.metrics.receivedSamples.WithLabelValues(profName).Observe(float64(len(p.Sample)))

			p.Normalize()

			// zip the data back into the buffer
			bw := bytes.NewBuffer(raw.RawProfile[:0])
			if _, err := p.WriteTo(bw); err != nil {
				return nil, err
			}
			p.Close()
			raw.RawProfile = bw.Bytes()
			// generate a unique profile ID before pushing.
			raw.ID = uuid.NewString()
		}
		profiles = append(profiles, &profileTracker{profile: series})
	}

	const maxExpectedReplicationSet = 5 // typical replication factor 3 plus one for inactive plus one for luck
	var descs [maxExpectedReplicationSet]ring.InstanceDesc

	samplesByIngester := map[string][]*profileTracker{}
	ingesterDescs := map[string]ring.InstanceDesc{}
	for i, key := range keys {
		replicationSet, err := d.ingestersRing.Get(key, ring.Write, descs[:0], nil, nil)
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
			localCtx = tenant.InjectTenantID(localCtx, tenantID)
			if sp := opentracing.SpanFromContext(ctx); sp != nil {
				localCtx = opentracing.ContextWithSpan(localCtx, sp)
			}
			d.sendProfiles(localCtx, ingester, samples, &tracker)
		}(ingesterDescs[ingester], samples)
	}
	select {
	case err := <-tracker.err:
		return nil, err
	case <-tracker.done:
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	case <-ctx.Done():
		return nil, ctx.Err()
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
		req.Msg.Series = append(req.Msg.Series, p.profile)
	}

	_, err = c.(PushClient).Push(ctx, req)
	return err
}

type profileTracker struct {
	profile     *pushv1.RawProfileSeries
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

func labelsString(ls []*commonv1.LabelPair) string {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, l := range ls {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(l.Value))
	}
	b.WriteByte('}')
	return b.String()
}

// TokenFor generates a token used for finding ingesters from ring
func TokenFor(tenantID, labels string) uint32 {
	h := fnv.New32()
	_, _ = h.Write([]byte(tenantID))
	_, _ = h.Write([]byte(labels))
	return h.Sum32()
}
