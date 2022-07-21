package distributor

import (
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"hash/fnv"
	"io/ioutil"
	"sort"
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
	"github.com/samber/lo"
	"github.com/weaveworks/common/user"
	"go.uber.org/atomic"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/ingester/clientpool"
	firemodel "github.com/grafana/fire/pkg/model"
)

type PushClient interface {
	Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)
}

// todo: move to non global metrics.
var clients = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "fire",
	Name:      "distributor_ingester_clients",
	Help:      "The current number of ingester clients.",
})

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

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, reg prometheus.Registerer, logger log.Logger) (*Distributor, error) {
	d := &Distributor{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		pool:          clientpool.NewPool(cfg.PoolConfig, ingestersRing, factory, clients, logger),
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

		// todo pool readers/writer
		gzipReader *gzip.Reader
		gzipWriter *gzip.Writer
		err        error
		br         = bytes.NewReader(nil)
	)

	for _, series := range req.Msg.Series {
		// todo propagate tenantID.
		keys = append(keys, TokenFor("", labelsString(series.Labels)))
		profName := firemodel.Labels(series.Labels).Get(scrape.ProfileName)
		for _, raw := range series.Samples {
			d.metrics.receivedCompressedBytes.WithLabelValues(profName).Observe(float64(len(raw.RawProfile)))
			br.Reset(raw.RawProfile)
			if gzipReader == nil {
				gzipReader, err = gzip.NewReader(br)
				if err != nil {
					return nil, errors.Wrap(err, "gzip reader")
				}
			} else {
				if err := gzipReader.Reset(br); err != nil {
					return nil, errors.Wrap(err, "gzip reset")
				}
			}
			data, err := ioutil.ReadAll(gzipReader)
			if err != nil {
				return nil, errors.Wrap(err, "gzip read all")
			}
			d.metrics.receivedDecompressedBytes.WithLabelValues(profName).Observe(float64(len(data)))
			p := profilev1.ProfileFromVTPool()
			if err := p.UnmarshalVT(data); err != nil {
				return nil, err
			}

			p = sanitizeProfile(p)

			// reuse the data buffer if possible
			size := p.SizeVT()
			if cap(data) < size {
				data = make([]byte, size)
			}
			n, err := p.MarshalToVT(data)
			if err != nil {
				return nil, err
			}
			p.ReturnToVTPool()
			data = data[:n]

			// zip the data back into the buffer
			bw := bytes.NewBuffer(raw.RawProfile[:0])
			if gzipWriter == nil {
				gzipWriter = gzip.NewWriter(bw)
			} else {
				gzipWriter.Reset(bw)
			}
			if _, err := gzipWriter.Write(data); err != nil {
				return nil, errors.Wrap(err, "gzip write")
			}
			if err := gzipWriter.Close(); err != nil {
				return nil, errors.Wrap(err, "gzip close")
			}
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

func sanitizeProfile(p *profilev1.Profile) *profilev1.Profile {
	// first pass remove samples that have no value.
	var removedSamples []*profilev1.Sample

	p.Sample = RemoveInPlace(p.Sample, func(s *profilev1.Sample) bool {
		for j := 0; j < len(s.Value); j++ {
			if s.Value[j] != 0 {
				s.Label = RemoveInPlace(s.Label, func(l *profilev1.Label) bool {
					// remove labels block "bytes" as it's redundant.
					if l.Num != 0 && l.Key != 0 &&
						p.StringTable[l.Key] == "bytes" {
						return true
					}
					return false
				})

				return false
			}
		}
		// all values are 0, remove the sample.
		removedSamples = append(removedSamples, s)
		return true
	})

	if len(removedSamples) == 0 {
		return p
	}
	// remove all data not used anymore.
	var removedLocationTotal int
	for _, s := range removedSamples {
		removedLocationTotal = len(s.LocationId)
	}
	removedLocationIds := make([]uint64, 0, removedLocationTotal)

	for _, s := range removedSamples {
		removedLocationIds = append(removedLocationIds, s.LocationId...)
	}
	removedLocationIds = lo.Uniq(removedLocationIds)

	// figure which removed Locations IDs are not used.
	for _, s := range p.Sample {
		for _, l := range s.LocationId {
			removedLocationIds = RemoveInPlace(removedLocationIds, func(locID uint64) bool {
				return l == locID
			})
		}
	}
	if len(removedLocationIds) == 0 {
		return p
	}
	var removedFunctionIds []uint64
	// remove the locations that are not used anymore.
	p.Location = RemoveInPlace(p.Location, func(loc *profilev1.Location) bool {
		if lo.Contains(removedLocationIds, loc.Id) {
			for _, l := range loc.Line {
				removedFunctionIds = append(removedFunctionIds, l.FunctionId)
			}
			return true
		}
		return false
	})

	if len(removedFunctionIds) == 0 {
		return p
	}
	removedFunctionIds = lo.Uniq(removedFunctionIds)
	// figure which removed Function IDs are not used.
	for _, l := range p.Location {
		for _, f := range l.Line {
			removedFunctionIds = RemoveInPlace(removedFunctionIds, func(fnID uint64) bool {
				// that ID is used in another location, remove it.
				return f.FunctionId == fnID
			})
		}
	}
	var removedNames []int64
	// remove the functions that are not used anymore.
	p.Function = RemoveInPlace(p.Function, func(fn *profilev1.Function) bool {
		if lo.Contains(removedFunctionIds, fn.Id) {
			removedNames = append(removedNames, fn.Name, fn.SystemName, fn.Filename)
			return true
		}
		return false
	})

	if len(removedNames) == 0 {
		return p
	}
	removedNames = lo.Uniq(removedNames)
	// remove names that are still used.
	forAllRefName(p, func(idx *int64) {
		removedNames = RemoveInPlace(removedNames, func(name int64) bool {
			return *idx == name
		})
	})
	if len(removedNames) == 0 {
		return p
	}
	// Sort to remove in order.
	sort.Slice(removedNames, func(i, j int) bool { return removedNames[i] < removedNames[j] })
	// remove the names that are not used anymore.
	p.StringTable = lo.Reject(p.StringTable, func(_ string, i int) bool {
		return lo.Contains(removedNames, int64(i))
	})

	// Now shift all indices [0,1,2,3,4,5,6]
	// if we removed [1,2,5] then we need to shift [3,4] to [1,2] and [6] to [3]
	// Basically we need to shift all indices that are greater than the removed index by the amount of removed indices.
	forAllRefName(p, func(idx *int64) {
		var shift int64
		for i := 0; i < len(removedNames); i++ {
			if *idx > removedNames[i] {
				shift++
				continue
			}
			break
		}
		*idx -= shift
	})

	return p
}

func forAllRefName(p *profilev1.Profile, fn func(*int64)) {
	fn(&p.DropFrames)
	fn(&p.KeepFrames)
	fn(&p.PeriodType.Type)
	fn(&p.PeriodType.Unit)
	for _, st := range p.SampleType {
		fn(&st.Type)
		fn(&st.Unit)
	}
	for _, m := range p.Mapping {
		fn(&m.Filename)
		fn(&m.BuildId)
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			fn(&l.Key)
			fn(&l.Num)
			fn(&l.NumUnit)
		}
	}
	for _, f := range p.Function {
		fn(&f.Name)
		fn(&f.SystemName)
		fn(&f.Filename)
	}
	for i := 0; i < len(p.Comment); i++ {
		fn(&p.Comment[i])
	}
}

// RemoveInPlace removes all elements from a slice that match the given predicate.
// Does not allocate a new slice.
func RemoveInPlace[T any](collection []T, predicate func(T) bool) []T {
	i := 0
	for _, x := range collection {
		if !predicate(x) {
			collection[i] = x
			i++
		}
	}
	return collection[:i]
}
