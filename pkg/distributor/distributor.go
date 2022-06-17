package distributor

import (
	"bytes"
	"context"
	"flag"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/weaveworks/common/user"
	"go.uber.org/atomic"

	"github.com/grafana/fire/pkg/gen/ingester/v1/ingestv1connect"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
)

// Config for a Distributor.
type Config struct {
	// Distributors ring
	DistributorRing RingConfig `yaml:"ring,omitempty"`
	PushTimeout     time.Duration
	PoolConfig      PoolConfig `yaml:"pool_config,omitempty"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.DistributorRing.RegisterFlags(fs)
	cfg.PoolConfig.RegisterFlags(fs)
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
}

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, logger log.Logger) (*Distributor, error) {
	if factory == nil {
		factory = PoolFactory
	}
	d := &Distributor{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		pool:          NewPool(cfg.PoolConfig, ingestersRing, factory, logger),
	}
	var err error
	d.subservices, err = services.NewManager(d.pool)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	d.subservicesWatcher = services.NewFailureWatcher()
	d.subservicesWatcher.WatchManager(d.subservices)
	d.Service = services.NewBasicService(d.starting, d.running, d.stopping)
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
	var (
		keys     = make([]uint32, 0, len(req.Msg.Series))
		profiles = make([]*profileTracker, 0, len(req.Msg.Series))
	)

	for _, series := range req.Msg.Series {
		// todo propagate tenantID.
		keys = append(keys, TokenFor("", labelsString(series.Labels)))
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
			localCtx = user.InjectOrgID(localCtx, "")
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

	_, err = c.(ingestv1connect.IngesterClient).Push(ctx, req)
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

func labelsString(ls []*pushv1.LabelPair) string {
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
