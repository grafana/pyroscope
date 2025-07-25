package segmentwriter

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/backoff"
	"github.com/oklog/ulid/v2"
	"github.com/opentracing/opentracing-go"
	"github.com/thanos-io/objstore"
	"golang.org/x/exp/maps"
	"golang.org/x/time/rate"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/pprofsplit"
	pprofmodel "github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/segmentwriter/memdb"
	"github.com/grafana/pyroscope/pkg/util/retry"
)

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

	metrics             *segmentMetrics
	headMetrics         *memdb.HeadMetrics
	hedgedUploadLimiter *rate.Limiter
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
	loopWG := new(sync.WaitGroup)
	ticker := time.NewTicker(sh.sw.config.SegmentDuration)
	defer func() {
		ticker.Stop()
		// Blocking here to make sure no asynchronous code is executed on this shard once loop exits
		// This is mostly needed to fix a race in our integration tests
		loopWG.Wait()
	}()
	for {
		select {
		case <-ticker.C:
			sh.flushSegment(context.Background(), loopWG)
		case <-ctx.Done():
			sh.flushSegment(context.Background(), loopWG)
			return
		}
	}
}

func (sh *shard) flushSegment(ctx context.Context, wg *sync.WaitGroup) {
	sh.mu.Lock()
	s := sh.segment
	sh.segment = sh.sw.newSegment(sh, s.shard, sh.logger)
	sh.mu.Unlock()

	wg.Add(1)
	go func() { // not blocking next ticks in case metastore/s3 latency is high
		defer wg.Done()
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
				"heads-count", len(s.datasets),
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
	sw.hedgedUploadLimiter = rate.NewLimiter(rate.Limit(sw.config.UploadHedgeRateMax), int(sw.config.UploadHedgeRateBurst))
	sw.ctx, sw.cancel = context.WithCancel(context.Background())
	flushWorkers := runtime.GOMAXPROCS(-1)
	if config.FlushConcurrency > 0 {
		flushWorkers = int(config.FlushConcurrency)
	}
	sw.pool.run(max(minFlushConcurrency, flushWorkers))
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

func (sw *segmentsWriter) stop() {
	sw.logger.Log("msg", "stopping segments writer")
	sw.cancel()
	sw.shardsLock.Lock()
	defer sw.shardsLock.Unlock()
	for _, s := range sw.shards {
		s.wg.Wait()
	}
	sw.pool.stop()
	sw.logger.Log("msg", "segments writer stopped")
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
		datasets: make(map[datasetKey]*dataset),
		sw:       sw,
		sh:       sh,
		shard:    sk,
		sshard:   sshard,
		doneChan: make(chan struct{}),
	}
	return s
}

func (s *segment) flush(ctx context.Context) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "segment.flush", opentracing.Tags{
		"block_id": s.ulid.String(),
		"datasets": len(s.datasets),
		"shard":    s.shard,
	})
	defer span.Finish()

	t1 := time.Now()
	defer func() {
		if err != nil {
			s.flushErrMutex.Lock()
			s.flushErr = err
			s.flushErrMutex.Unlock()
		}
		close(s.doneChan)
		s.sw.metrics.flushSegmentDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	}()

	stream := s.flushHeads(ctx)
	s.debuginfo.movedHeads = len(stream.heads)
	if len(stream.heads) == 0 {
		return nil
	}

	// TODO(kolesnikovae): Use buffer pool for blockData.
	blockData, blockMeta, err := s.flushBlock(stream)
	if err != nil {
		return fmt.Errorf("failed to flush block %s: %w", s.ulid.String(), err)
	}
	if err = s.sw.uploadBlock(ctx, blockData, blockMeta, s); err != nil {
		return fmt.Errorf("failed to upload block %s: %w", s.ulid.String(), err)
	}
	if err = s.sw.storeMetadata(ctx, blockMeta, s); err != nil {
		return fmt.Errorf("failed to store meta %s: %w", s.ulid.String(), err)
	}

	return nil
}

func (s *segment) flushBlock(stream flushStream) ([]byte, *metastorev1.BlockMeta, error) {
	start := time.Now()
	hostname, _ := os.Hostname()

	stringTable := metadata.NewStringTable()
	meta := &metastorev1.BlockMeta{
		FormatVersion:   1,
		Id:              s.ulid.String(),
		Tenant:          0,
		Shard:           uint32(s.shard),
		CompactionLevel: 0,
		CreatedBy:       stringTable.Put(hostname),
		MinTime:         math.MaxInt64,
		MaxTime:         0,
		Size:            0,
		Datasets:        make([]*metastorev1.Dataset, 0, len(stream.heads)),
	}

	blockFile := bytes.NewBuffer(nil)

	w := &writerOffset{Writer: blockFile}
	for stream.Next() {
		f := stream.At()
		// TODO(kolesnikovae): Build dataset index for the tenant.
		//   Note that the heads are flushed concurrently, so we cannot build
		//   during head flush. I'd prefer to delegate it to the head itself:
		//      WriteDatasetIndex(w *memindex.Writer, idx uint32)
		//   Tenant datasets follow sequentially; when all tenant datasets
		//   are flushed, we can build the index and create a metadata
		//   entry for it.
		ds := concatSegmentHead(f, w, stringTable)
		meta.MinTime = min(meta.MinTime, ds.MinTime)
		meta.MaxTime = max(meta.MaxTime, ds.MaxTime)
		meta.Datasets = append(meta.Datasets, ds)
		s.sw.metrics.headSizeBytes.WithLabelValues(s.sshard, f.dataset.key.tenant).Observe(float64(ds.Size))
	}

	meta.StringTable = stringTable.Strings
	meta.MetadataOffset = uint64(w.offset)
	if err := metadata.Encode(w, meta); err != nil {
		return nil, nil, fmt.Errorf("failed to encode metadata: %w", err)
	}
	meta.Size = uint64(w.offset)
	s.debuginfo.flushBlockDuration = time.Since(start)
	return blockFile.Bytes(), meta, nil
}

type writerOffset struct {
	io.Writer
	offset int64
}

func (w *writerOffset) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	w.offset += int64(n)
	return n, err
}

func concatSegmentHead(f *datasetFlush, w *writerOffset, s *metadata.StringTable) *metastorev1.Dataset {
	tenantServiceOffset := w.offset

	ptypes := f.flushed.Meta.ProfileTypeNames

	offsets := []uint64{0, 0, 0}

	offsets[0] = uint64(w.offset)
	_, _ = w.Write(f.flushed.Profiles)

	offsets[1] = uint64(w.offset)
	_, _ = w.Write(f.flushed.Index)

	offsets[2] = uint64(w.offset)
	_, _ = w.Write(f.flushed.Symbols)

	tenantServiceSize := w.offset - tenantServiceOffset

	ds := &metastorev1.Dataset{
		Tenant:  s.Put(f.dataset.key.tenant),
		Name:    s.Put(f.dataset.key.service),
		MinTime: f.flushed.Meta.MinTimeNanos / 1e6,
		MaxTime: f.flushed.Meta.MaxTimeNanos / 1e6,
		Size:    uint64(tenantServiceSize),
		//  - 0: profiles.parquet
		//  - 1: index.tsdb
		//  - 2: symbols.symdb
		TableOfContents: offsets,
		Labels:          nil,
	}

	lb := metadata.NewLabelBuilder(s)
	for _, profileType := range ptypes {
		lb.WithLabelSet(model.LabelNameServiceName, f.dataset.key.service, model.LabelNameProfileType, profileType)
	}

	if f.flushed.Unsymbolized {
		lb.WithLabelSet(model.LabelNameServiceName, f.dataset.key.service, metadata.LabelNameUnsymbolized, "true")
	}

	// Other optional labels:
	// lb.WithLabelSet("label_name", "label_value", ...)
	ds.Labels = lb.Build()

	return ds
}

func (s *segment) flushHeads(ctx context.Context) flushStream {
	heads := maps.Values(s.datasets)
	slices.SortFunc(heads, func(a, b *dataset) int {
		return a.key.compare(b.key)
	})

	stream := make([]*datasetFlush, len(heads))
	for i := range heads {
		f := &datasetFlush{
			dataset: heads[i],
			done:    make(chan struct{}),
		}
		stream[i] = f
		s.sw.pool.do(func() {
			defer close(f.done)
			flushed, err := s.flushDataset(ctx, f.dataset)
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
			f.flushed = flushed
		})
	}

	return flushStream{heads: stream}
}

type flushStream struct {
	heads []*datasetFlush
	cur   *datasetFlush
	n     int
}

func (s *flushStream) At() *datasetFlush { return s.cur }

func (s *flushStream) Next() bool {
	for s.n < len(s.heads) {
		f := s.heads[s.n]
		s.n++
		<-f.done
		if f.flushed != nil {
			s.cur = f
			return true
		}
	}
	return false
}

func (s *segment) flushDataset(ctx context.Context, e *dataset) (*memdb.FlushedHead, error) {
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

type datasetKey struct {
	tenant  string
	service string
}

func (k datasetKey) compare(x datasetKey) int {
	if k.tenant != x.tenant {
		return strings.Compare(k.tenant, x.tenant)
	}
	return strings.Compare(k.service, x.service)
}

type dataset struct {
	key  datasetKey
	sw   *segmentsWriter
	once sync.Once
	head *memdb.Head
}

func newDataset(k datasetKey, sw *segmentsWriter) *dataset { return &dataset{key: k, sw: sw} }

func (d *dataset) initHead() *memdb.Head {
	d.once.Do(func() {
		d.head = memdb.NewHead(d.sw.headMetrics)
	})
	return d.head
}

type datasetFlush struct {
	dataset *dataset
	flushed *memdb.FlushedHead
	done    chan struct{}
}

type segment struct {
	ulid             ulid.ULID
	shard            shardKey
	sshard           string
	inFlightProfiles sync.WaitGroup

	datasetsLock sync.Mutex
	datasets     map[datasetKey]*dataset

	logger log.Logger
	sw     *segmentsWriter

	// TODO(kolesnikovae): Revisit.
	doneChan      chan struct{}
	flushErr      error
	flushErrMutex sync.Mutex

	debuginfo struct {
		movedHeads         int
		waitInflight       time.Duration
		flushHeadsDuration time.Duration
		flushBlockDuration time.Duration
		storeMetaDuration  time.Duration
	}

	// TODO(kolesnikovae): Naming.
	sh *shard
}

type segmentIngest interface {
	ingest(tenantID string, p *profilev1.Profile, id uuid.UUID, labels []*typesv1.LabelPair, annotations []*typesv1.ProfileAnnotation)
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

func (s *segment) ingest(tenantID string, p *profilev1.Profile, id uuid.UUID, labels []*typesv1.LabelPair, annotations []*typesv1.ProfileAnnotation) {
	// TODO(kolesnikovae): Refactor: profile split should be moved inside the
	//   dataset.Ingest: we want to do it together with / instead of creation
	//   of the internal representation (InMemoryProfile).
	//   symbols.WriteProfileSymbols should be replaced with something more
	//   suitable (see comment) – we want to avoid allocating intermediate
	//   objects that are used only temporarily.
	//   Many sample series refer to same symbols, so we can avoid extra
	//   processing and index symbols just once: at this point we know that
	//   all samples are to be stored, and all the referred symbols need to
	//   be indexed. This will require quite a bit of refactoring, but it's
	//   worth it.
	serviceName := model.Labels(labels).Get(model.LabelNameServiceName)
	ds := s.datasetForIngest(datasetKey{tenant: tenantID, service: serviceName})
	appender := &sampleAppender{dataset: ds.initHead(), profile: p, id: id, annotations: annotations}
	// Relabeling rules cannot be applied here: it should be done before the
	// ingestion, in distributors. Otherwise, it may change the distribution
	// key, including the "service_name" label, which we use to determine the
	// profile target dataset.
	// TODO: Replace with pprof.GroupSamples
	_ = pprofsplit.VisitSampleSeries(p, labels, nil, appender)
	s.sw.metrics.segmentIngestBytes.WithLabelValues(s.sshard, tenantID).Observe(float64(p.SizeVT()))
}

type sampleAppender struct {
	id          uuid.UUID
	dataset     *memdb.Head
	profile     *profilev1.Profile
	exporter    *pprofmodel.SampleExporter
	annotations []*typesv1.ProfileAnnotation
}

func (v *sampleAppender) VisitProfile(labels model.Labels) {
	v.dataset.Ingest(v.profile, v.id, labels, v.annotations)
}

func (v *sampleAppender) VisitSampleSeries(labels model.Labels, samples []*profilev1.Sample) {
	if v.exporter == nil {
		v.exporter = pprofmodel.NewSampleExporter(v.profile)
	}
	var n profilev1.Profile
	v.exporter.ExportSamples(&n, samples)
	v.dataset.Ingest(&n, v.id, labels, v.annotations)
}

func (v *sampleAppender) ValidateLabels(model.Labels) error { return nil }

func (v *sampleAppender) Discarded(_, _ int) {}

func (s *segment) datasetForIngest(k datasetKey) *dataset {
	s.datasetsLock.Lock()
	ds, ok := s.datasets[k]
	if !ok {
		ds = newDataset(k, s.sw)
		s.datasets[k] = ds
	}
	s.datasetsLock.Unlock()
	return ds
}

func (sw *segmentsWriter) uploadBlock(ctx context.Context, blockData []byte, meta *metastorev1.BlockMeta, s *segment) error {
	uploadStart := time.Now()
	var err error
	defer func() {
		sw.metrics.segmentUploadDuration.
			WithLabelValues(statusLabelValue(err)).
			Observe(time.Since(uploadStart).Seconds())
	}()

	path := block.ObjectPath(meta)
	sw.metrics.segmentSizeBytes.
		WithLabelValues(s.sshard).
		Observe(float64(len(blockData)))

	if sw.config.UploadTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sw.config.UploadTimeout)
		defer cancel()
	}

	// To mitigate tail latency issues, we use a hedged upload strategy:
	// if the request is not completed within a certain time, we trigger
	// a second upload attempt. Upload errors are retried explicitly and
	// are included into the call duration.
	hedgedUpload := retry.Hedged[any]{
		Trigger: time.After(sw.config.UploadHedgeAfter),
		Call: func(ctx context.Context, hedge bool) (any, error) {
			retryConfig := backoff.Config{
				MinBackoff: sw.config.UploadMinBackoff,
				MaxBackoff: sw.config.UploadMaxBackoff,
				MaxRetries: sw.config.UploadMaxRetries,
			}
			var attemptErr error
			if hedge {
				if limitErr := sw.hedgedUploadLimiter.Wait(ctx); limitErr != nil {
					return nil, limitErr
				}
				// Hedged requests are not retried.
				retryConfig.MaxRetries = 0
				attemptStart := time.Now()
				defer func() {
					sw.metrics.segmentHedgedUploadDuration.
						WithLabelValues(statusLabelValue(attemptErr)).
						Observe(time.Since(attemptStart).Seconds())
				}()
			}
			// Retry on all errors.
			retries := backoff.New(ctx, retryConfig)
			for retries.Ongoing() {
				if attemptErr = sw.bucket.Upload(ctx, path, bytes.NewReader(blockData)); attemptErr == nil {
					break
				}
				retries.Wait()
			}
			return nil, attemptErr
		},
	}

	if _, err = hedgedUpload.Do(ctx); err != nil {
		return err
	}

	level.Debug(sw.logger).Log("msg", "uploaded block", "path", path, "upload_duration", time.Since(uploadStart))
	return nil
}

func (sw *segmentsWriter) storeMetadata(ctx context.Context, meta *metastorev1.BlockMeta, s *segment) error {
	start := time.Now()
	var err error
	defer func() {
		sw.metrics.storeMetadataDuration.
			WithLabelValues(statusLabelValue(err)).
			Observe(time.Since(start).Seconds())
		s.debuginfo.storeMetaDuration = time.Since(start)
	}()

	mdCtx := ctx
	if sw.config.MetadataUpdateTimeout > 0 {
		var cancel context.CancelFunc
		mdCtx, cancel = context.WithTimeout(mdCtx, sw.config.MetadataUpdateTimeout)
		defer cancel()
	}

	if _, err = sw.metastore.AddBlock(mdCtx, &metastorev1.AddBlockRequest{Block: meta}); err == nil {
		return nil
	}

	level.Error(s.logger).Log("msg", "failed to store meta in metastore", "err", err)
	if !sw.config.MetadataDLQEnabled {
		return err
	}

	defer func() {
		sw.metrics.storeMetadataDLQ.WithLabelValues(statusLabelValue(err)).Inc()
	}()

	if err = s.sw.storeMetadataDLQ(ctx, meta); err == nil {
		return nil
	}

	level.Error(s.logger).Log("msg", "metastore fallback failed", "err", err)
	return err
}

func (sw *segmentsWriter) storeMetadataDLQ(ctx context.Context, meta *metastorev1.BlockMeta) error {
	metadataBytes, err := meta.MarshalVT()
	if err != nil {
		return err
	}
	return sw.bucket.Upload(ctx, block.MetadataDLQObjectPath(meta), bytes.NewReader(metadataBytes))
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
