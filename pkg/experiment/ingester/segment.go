package ingester

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/thanos-io/objstore"
	"golang.org/x/exp/maps"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/memdb"
	"github.com/grafana/pyroscope/pkg/model"
	pprofsplit "github.com/grafana/pyroscope/pkg/model/pprof_split"
	pprofmodel "github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/util/math"
	"github.com/grafana/pyroscope/pkg/validation"
)

var ErrMetastoreDLQFailed = fmt.Errorf("failed to store block metadata in DLQ")

type shardKey uint32

type segmentsWriter struct {
	config    Config
	limits    Limits
	logger    log.Logger
	bucket    objstore.Bucket
	metastore metastorev1.IndexServiceClient

	shards     map[shardKey]*shard
	shardsLock sync.RWMutex
	pool       workerPool

	ctx    context.Context
	cancel context.CancelFunc

	metrics     *segmentMetrics
	headMetrics *memdb.HeadMetrics
}

type shard struct {
	wg        sync.WaitGroup
	logger    log.Logger
	concatBuf []byte
	sw        *segmentsWriter
	mu        sync.RWMutex
	segment   *segment
}

func (sh *shard) ingest(fn func(head segmentIngest)) segmentWaitFlushed {
	sh.mu.RLock()
	s := sh.segment
	s.inFlightProfiles.Add(1)
	sh.mu.RUnlock()
	defer s.inFlightProfiles.Done()
	fn(s)
	return s
}

func (sh *shard) loop(ctx context.Context) {
	ticker := time.NewTicker(sh.sw.config.SegmentDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sh.flushSegment(context.Background())
		case <-ctx.Done():
			sh.flushSegment(context.Background())
			return
		}
	}
}

func (sh *shard) flushSegment(ctx context.Context) {
	sh.mu.Lock()
	s := sh.segment
	sh.segment = sh.sw.newSegment(sh, s.shard, sh.logger)
	sh.mu.Unlock()

	go func() { // not blocking next ticks in case metastore/s3 latency is high
		t1 := time.Now()
		s.inFlightProfiles.Wait()
		s.debuginfo.waitInflight = time.Since(t1)

		err := s.flush(ctx)
		if err != nil {
			_ = level.Error(sh.sw.logger).Log("msg", "failed to flush segment", "err", err)
		}
		if s.debuginfo.movedHeads > 0 {
			_ = level.Debug(s.logger).Log("msg",
				"writing segment block done",
				"heads-count", len(s.heads),
				"heads-moved-count", s.debuginfo.movedHeads,
				"inflight-duration", s.debuginfo.waitInflight,
				"flush-heads-duration", s.debuginfo.flushHeadsDuration,
				"flush-block-duration", s.debuginfo.flushBlockDuration,
				"store-meta-duration", s.debuginfo.storeMetaDuration,
				"total-duration", time.Since(t1))
		}
	}()
}

func newSegmentWriter(l log.Logger, metrics *segmentMetrics, hm *memdb.HeadMetrics, config Config, limits Limits, bucket objstore.Bucket, metastoreClient metastorev1.IndexServiceClient) *segmentsWriter {
	sw := &segmentsWriter{
		limits:      limits,
		metrics:     metrics,
		headMetrics: hm,
		config:      config,
		logger:      l,
		bucket:      bucket,
		shards:      make(map[shardKey]*shard),
		metastore:   metastoreClient,
	}
	sw.ctx, sw.cancel = context.WithCancel(context.Background())
	// One worker per CPU core, but not less than 4.
	flushWorkers := runtime.GOMAXPROCS(-1)
	if config.FlushConcurrency > 0 {
		flushWorkers = int(config.FlushConcurrency)
	}
	sw.pool.run(max(4, flushWorkers))
	return sw
}

func (sw *segmentsWriter) ingest(shard shardKey, fn func(head segmentIngest)) (await segmentWaitFlushed) {
	sw.shardsLock.RLock()
	s, ok := sw.shards[shard]
	sw.shardsLock.RUnlock()
	if ok {
		return s.ingest(fn)
	}

	sw.shardsLock.Lock()
	s, ok = sw.shards[shard]
	if ok {
		sw.shardsLock.Unlock()
		return s.ingest(fn)
	}

	s = sw.newShard(shard)
	sw.shards[shard] = s
	sw.shardsLock.Unlock()
	return s.ingest(fn)
}

func (sw *segmentsWriter) stop() error {
	sw.logger.Log("msg", "stopping segments writer")
	sw.cancel()
	sw.shardsLock.Lock()
	defer sw.shardsLock.Unlock()
	for _, s := range sw.shards {
		s.wg.Wait()
	}
	sw.pool.stop()
	sw.logger.Log("msg", "segments writer stopped")
	return nil
}

func (sw *segmentsWriter) newShard(sk shardKey) *shard {
	sl := log.With(sw.logger, "shard", fmt.Sprintf("%d", sk))
	sh := &shard{
		sw:        sw,
		logger:    sl,
		concatBuf: make([]byte, 4*0x1000),
	}
	sh.segment = sw.newSegment(sh, sk, sl)
	sh.wg.Add(1)
	go func() {
		defer sh.wg.Done()
		sh.loop(sw.ctx)
	}()
	return sh
}

func (sw *segmentsWriter) newSegment(sh *shard, sk shardKey, sl log.Logger) *segment {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	sshard := fmt.Sprintf("%d", sk)
	s := &segment{
		logger:   log.With(sl, "segment-id", id.String()),
		ulid:     id,
		heads:    make(map[serviceKey]serviceHead),
		sw:       sw,
		sh:       sh,
		shard:    sk,
		sshard:   sshard,
		doneChan: make(chan struct{}),
	}
	return s
}

func (s *segment) flush(ctx context.Context) (err error) {
	t1 := time.Now()
	var heads []flushedServiceHead

	defer func() {
		if err != nil {
			s.flushErrMutex.Lock()
			s.flushErr = err
			s.flushErrMutex.Unlock()
		}
		close(s.doneChan)
		s.sw.metrics.flushSegmentDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	}()

	// TODO(kolesnikovae): Stream flushed heads to the next stage.
	pprof.Do(ctx, pprof.Labels("segment_op", "flush_heads"), func(ctx context.Context) {
		heads = s.flushHeads(ctx)
	})
	s.debuginfo.movedHeads = len(heads)
	if len(heads) == 0 {
		return nil
	}

	blockData, blockMeta, err := s.flushBlock(heads)
	if err != nil {
		return fmt.Errorf("failed to flush block %s: %w", s.ulid.String(), err)
	}
	if err = s.sw.uploadBlock(ctx, blockData, blockMeta, s); err != nil {
		return fmt.Errorf("failed to upload block %s: %w", s.ulid.String(), err)
	}
	if err = s.sw.storeMeta(ctx, blockMeta, s); err != nil {
		level.Error(s.logger).Log("msg", "failed to store meta in metastore", "err", err)
		if dlqErr := s.sw.storeMetaDLQ(ctx, blockMeta, s); dlqErr != nil {
			level.Error(s.logger).Log("msg", "metastore fallback failed", "err", dlqErr)
			return fmt.Errorf("failed to store meta %s: %w", s.ulid.String(), dlqErr)
		}
	}

	return nil
}

func (s *segment) flushBlock(heads []flushedServiceHead) ([]byte, *metastorev1.BlockMeta, error) {
	t1 := time.Now()
	hostname, _ := os.Hostname()

	stringTable := metadata.NewStringTable()
	meta := &metastorev1.BlockMeta{
		FormatVersion:   1,
		Id:              s.ulid.String(),
		Tenant:          0,
		Shard:           uint32(s.shard),
		CompactionLevel: 0,
		CreatedBy:       stringTable.Put(hostname),
		MinTime:         0,
		MaxTime:         0,
		Size:            0,
		Datasets:        make([]*metastorev1.Dataset, 0, len(heads)),
	}

	blockFile := bytes.NewBuffer(nil)

	w := withWriterOffset(blockFile)

	for i, e := range heads {
		svc, err := concatSegmentHead(e, w, stringTable)
		if err != nil {
			_ = level.Error(s.logger).Log("msg", "failed to concat segment head", "err", err)
			continue
		}
		if i == 0 {
			meta.MinTime = svc.MinTime
			meta.MaxTime = svc.MaxTime
		} else {
			meta.MinTime = math.Min(meta.MinTime, svc.MinTime)
			meta.MaxTime = math.Max(meta.MaxTime, svc.MaxTime)
		}
		s.sw.metrics.headSizeBytes.WithLabelValues(s.sshard, e.key.tenant).Observe(float64(svc.Size))
		meta.Datasets = append(meta.Datasets, svc)
	}

	meta.StringTable = stringTable.Strings
	meta.Size = uint64(w.offset)
	s.debuginfo.flushBlockDuration = time.Since(t1)
	return blockFile.Bytes(), meta, nil
}

func concatSegmentHead(e flushedServiceHead, w *writerOffset, s *metadata.StringTable) (*metastorev1.Dataset, error) {
	tenantServiceOffset := w.offset

	ptypes := e.head.Meta.ProfileTypeNames

	offsets := []uint64{0, 0, 0}

	offsets[0] = uint64(w.offset)
	_, _ = w.Write(e.head.Profiles)

	offsets[1] = uint64(w.offset)
	_, _ = w.Write(e.head.Index)

	offsets[2] = uint64(w.offset)
	_, _ = w.Write(e.head.Symbols)

	tenantServiceSize := w.offset - tenantServiceOffset

	ds := &metastorev1.Dataset{
		Tenant:  s.Put(e.key.tenant),
		Name:    s.Put(e.key.service),
		MinTime: e.head.Meta.MinTimeNanos / 1e6,
		MaxTime: e.head.Meta.MaxTimeNanos / 1e6,
		Size:    uint64(tenantServiceSize),
		//  - 0: profiles.parquet
		//  - 1: index.tsdb
		//  - 2: symbols.symdb
		TableOfContents: offsets,
		Labels:          nil,
	}

	lb := metadata.NewLabelBuilder(s).
		WithConstantPairs(model.LabelNameServiceName, e.key.service).
		WithLabelNames(model.LabelNameProfileType)

	for _, profileType := range ptypes {
		lb.CreateLabels(profileType)
	}

	ds.Labels = lb.Build()

	return ds, nil
}

func (s *segment) flushHeads(ctx context.Context) (moved []flushedServiceHead) {
	t1 := time.Now()
	defer func() {
		s.sw.metrics.flushHeadsDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
		s.debuginfo.flushHeadsDuration = time.Since(t1)
	}()

	heads := maps.Values(s.heads)
	moved = make([]flushedServiceHead, len(heads))
	slices.SortFunc(heads, func(a, b serviceHead) int {
		return a.key.compare(b.key)
	})

	var flush sync.WaitGroup
	flush.Add(len(heads))
	for i, h := range heads {
		s.sw.pool.do(func() {
			defer flush.Done()
			flushed, err := s.flushHead(ctx, h)
			if err != nil {
				level.Error(s.logger).Log("msg", "failed to flush head", "err", err)
				return
			}
			if flushed == nil {
				level.Debug(s.logger).Log("msg", "skipping nil head")
				return
			}
			if flushed.Meta.NumSamples == 0 {
				level.Debug(s.logger).Log("msg", "skipping empty head")
				return
			}
			moved[i] = flushedServiceHead{
				key:  h.key,
				head: flushed,
			}
		})
	}

	flush.Wait()
	moved = slices.DeleteFunc(moved, func(x flushedServiceHead) bool {
		return x.head == nil
	})

	return moved
}

func (s *segment) flushHead(ctx context.Context, e serviceHead) (*memdb.FlushedHead, error) {
	th := time.Now()
	flushed, err := e.head.Flush(ctx)
	if err != nil {
		s.sw.metrics.flushServiceHeadDuration.WithLabelValues(s.sshard, e.key.tenant).Observe(time.Since(th).Seconds())
		s.sw.metrics.flushServiceHeadError.WithLabelValues(s.sshard, e.key.tenant).Inc()
		return nil, fmt.Errorf("failed to flush head : %w", err)
	}
	s.sw.metrics.flushServiceHeadDuration.WithLabelValues(s.sshard, e.key.tenant).Observe(time.Since(th).Seconds())
	level.Debug(s.logger).Log(
		"msg", "flushed head",
		"tenant", e.key.tenant,
		"service", e.key.service,
		"profiles", flushed.Meta.NumProfiles,
		"profiletypes", fmt.Sprintf("%v", flushed.Meta.ProfileTypeNames),
		"mintime", flushed.Meta.MinTimeNanos,
		"maxtime", flushed.Meta.MaxTimeNanos,
		"head-flush-duration", time.Since(th).String(),
	)
	return flushed, nil
}

type serviceKey struct {
	tenant  string
	service string
}

func (k serviceKey) compare(x serviceKey) int {
	if k.tenant != x.tenant {
		return strings.Compare(k.tenant, x.tenant)
	}
	return strings.Compare(k.service, x.service)
}

type serviceHead struct {
	key  serviceKey
	head *memdb.Head
}

type flushedServiceHead struct {
	key  serviceKey
	head *memdb.FlushedHead
}

type segment struct {
	ulid             ulid.ULID
	shard            shardKey
	sshard           string
	inFlightProfiles sync.WaitGroup
	heads            map[serviceKey]serviceHead
	headsLock        sync.RWMutex
	sw               *segmentsWriter
	doneChan         chan struct{}
	flushErr         error
	flushErrMutex    sync.Mutex
	logger           log.Logger

	debuginfo struct {
		movedHeads         int
		waitInflight       time.Duration
		flushHeadsDuration time.Duration
		flushBlockDuration time.Duration
		storeMetaDuration  time.Duration
	}
	sh      *shard
	counter int64
}

type segmentIngest interface {
	ingest(tenantID string, p *profilev1.Profile, id uuid.UUID, labels []*typesv1.LabelPair)
}

type segmentWaitFlushed interface {
	waitFlushed(ctx context.Context) error
}

func (s *segment) waitFlushed(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("waitFlushed: %s %w", s.ulid.String(), ctx.Err())
	case <-s.doneChan:
		s.flushErrMutex.Lock()
		res := s.flushErr
		s.flushErrMutex.Unlock()
		return res
	}
}

func (s *segment) ingest(tenantID string, p *profilev1.Profile, id uuid.UUID, labels []*typesv1.LabelPair) {
	k := serviceKey{
		tenant:  tenantID,
		service: model.Labels(labels).Get(model.LabelNameServiceName),
	}
	size := p.SizeVT()
	rules := s.sw.limits.IngestionRelabelingRules(tenantID)
	usage := s.sw.limits.DistributorUsageGroups(tenantID).GetUsageGroups(tenantID, labels)
	appender := &sampleAppender{
		head:    s.headForIngest(k),
		profile: p,
		id:      id,
	}
	pprofsplit.VisitSampleSeries(p, labels, rules, appender)
	size -= appender.discardedBytes
	s.sw.metrics.segmentIngestBytes.WithLabelValues(s.sshard, tenantID).Observe(float64(size))
	usage.CountDiscardedBytes(string(validation.DroppedByRelabelRules), int64(appender.discardedBytes))
	// CountReceivedBytes is tracked in distributors.
}

type sampleAppender struct {
	id       uuid.UUID
	head     *memdb.Head
	profile  *profilev1.Profile
	exporter *pprofmodel.SampleExporter

	discardedProfiles int
	discardedBytes    int
}

func (v *sampleAppender) VisitProfile(labels []*typesv1.LabelPair) {
	v.head.Ingest(v.profile, v.id, labels)
}

func (v *sampleAppender) VisitSampleSeries(labels []*typesv1.LabelPair, samples []*profilev1.Sample) {
	if v.exporter == nil {
		v.exporter = pprofmodel.NewSampleExporter(v.profile)
	}
	var n profilev1.Profile
	v.exporter.ExportSamples(&n, samples)
	v.head.Ingest(&n, v.id, labels)
}

func (v *sampleAppender) Discarded(profiles, bytes int) {
	v.discardedProfiles += profiles
	v.discardedBytes += bytes
}

func (s *segment) headForIngest(k serviceKey) *memdb.Head {
	s.headsLock.RLock()
	h, ok := s.heads[k]
	s.headsLock.RUnlock()
	if ok {
		return h.head
	}

	s.headsLock.Lock()
	defer s.headsLock.Unlock()
	h, ok = s.heads[k]
	if ok {
		return h.head
	}

	nh := memdb.NewHead(s.sw.headMetrics)

	s.heads[k] = serviceHead{
		key:  k,
		head: nh,
	}

	return nh
}

func (sw *segmentsWriter) uploadBlock(ctx context.Context, blockData []byte, meta *metastorev1.BlockMeta, s *segment) error {
	t1 := time.Now()
	defer func() {
		sw.metrics.blockUploadDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	}()
	sw.metrics.segmentBlockSizeBytes.WithLabelValues(s.sshard).Observe(float64(len(blockData)))

	blockPath := block.ObjectPath(meta)

	if err := sw.bucket.Upload(ctx, blockPath, bytes.NewReader(blockData)); err != nil {
		return err
	}
	sw.logger.Log("msg", "uploaded block", "path", blockPath, "upload_duration", time.Since(t1))
	return nil
}

func (sw *segmentsWriter) storeMeta(ctx context.Context, meta *metastorev1.BlockMeta, s *segment) error {
	t1 := time.Now()
	defer func() {
		sw.metrics.storeMetaDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
		s.debuginfo.storeMetaDuration = time.Since(t1)
	}()
	_, err := sw.metastore.AddBlock(ctx, &metastorev1.AddBlockRequest{Block: meta})
	if err != nil {
		sw.metrics.storeMetaErrors.WithLabelValues(s.sshard).Inc()
	}
	return err
}

func (sw *segmentsWriter) storeMetaDLQ(ctx context.Context, meta *metastorev1.BlockMeta, s *segment) error {
	metaBlob, err := meta.MarshalVT()
	if err != nil {
		sw.metrics.storeMetaDLQ.WithLabelValues(s.sshard, "err").Inc()
		return err
	}
	fullPath := block.MetadataDLQObjectPath(meta)
	if err = sw.bucket.Upload(ctx, fullPath, bytes.NewReader(metaBlob)); err != nil {
		sw.metrics.storeMetaDLQ.WithLabelValues(s.sshard, "err").Inc()
		return fmt.Errorf("%w, %w", ErrMetastoreDLQFailed, err)
	}
	sw.metrics.storeMetaDLQ.WithLabelValues(s.sshard, "OK").Inc()
	return nil
}

type workerPool struct {
	workers sync.WaitGroup
	jobs    chan func()
}

func (p *workerPool) run(n int) {
	if p.jobs != nil {
		return
	}
	p.jobs = make(chan func())
	p.workers.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer p.workers.Done()
			for job := range p.jobs {
				job()
			}
		}()
	}
}

// do must not be called after stop.
func (p *workerPool) do(job func()) {
	p.jobs <- job
}

func (p *workerPool) stop() {
	close(p.jobs)
	p.workers.Wait()
}
