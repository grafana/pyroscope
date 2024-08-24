package ingester

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/memdb"
	"path"
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

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util/math"
)

const pathSegments = "segments"
const pathDLQ = "dlq"
const pathAnon = tenant.DefaultTenantID
const pathBlock = "block.bin"

type shardKey uint32

type segmentWriterConfig struct {
	segmentDuration time.Duration
}

type segmentsWriter struct {
	segmentDuration time.Duration

	l               log.Logger
	bucket          objstore.Bucket
	metastoreClient metastorev1.MetastoreServiceClient

	shards     map[shardKey]*shard
	shardsLock sync.RWMutex

	cancelCtx context.Context
	cancel    context.CancelFunc

	metrics     *segmentMetrics
	headMetrics *memdb.HeadMetrics
}

type shard struct {
	sw          *segmentsWriter
	current     *segment
	currentLock sync.RWMutex
	wg          sync.WaitGroup
	l           log.Logger
	concatBuf   []byte
}

func (sh *shard) ingest(fn func(head segmentIngest) error) (segmentWaitFlushed, error) {
	sh.currentLock.RLock()
	s := sh.current
	s.inFlightProfiles.Add(1)
	sh.currentLock.RUnlock()
	defer s.inFlightProfiles.Done()
	return s, fn(s)
}

func (sh *shard) loop(ctx context.Context) {
	ticker := time.NewTicker(sh.sw.segmentDuration)
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
	sh.currentLock.Lock()
	s := sh.current
	sh.current = sh.sw.newSegment(sh, s.shard, sh.l)
	sh.currentLock.Unlock()

	go func() { // not blocking next ticks in case metastore/s3 latency is high
		t1 := time.Now()
		s.inFlightProfiles.Wait()
		s.debuginfo.waitInflight = time.Since(t1)

		err := s.flush(ctx)
		if err != nil {
			_ = level.Error(sh.sw.l).Log("msg", "failed to flush segment", "err", err)
		}
		if s.debuginfo.movedHeads > 0 {
			_ = level.Debug(s.l).Log("msg",
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

func newSegmentWriter(l log.Logger, metrics *segmentMetrics, hm *memdb.HeadMetrics, cfg segmentWriterConfig, bucket objstore.Bucket, metastoreClient metastorev1.MetastoreServiceClient) *segmentsWriter {
	ctx, cancelFunc := context.WithCancel(context.Background())
	sw := &segmentsWriter{
		metrics:         metrics,
		headMetrics:     hm,
		segmentDuration: cfg.segmentDuration,
		l:               l,
		bucket:          bucket,
		shards:          make(map[shardKey]*shard),
		metastoreClient: metastoreClient,
		cancel:          cancelFunc,
		cancelCtx:       ctx,
	}

	return sw
}

func (sw *segmentsWriter) ingest(shard shardKey, fn func(head segmentIngest) error) (await segmentWaitFlushed, err error) {
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

func (sw *segmentsWriter) Stop() error {
	sw.l.Log("msg", "stopping segments writer")
	sw.cancel()
	sw.shardsLock.Lock()
	defer sw.shardsLock.Unlock()
	for _, s := range sw.shards {
		s.wg.Wait()
	}
	sw.l.Log("msg", "segments writer stopped")

	return nil
}

func (sw *segmentsWriter) newShard(sk shardKey) *shard {
	sl := log.With(sw.l, "shard", fmt.Sprintf("%d", sk))
	sh := &shard{
		sw:        sw,
		l:         sl,
		concatBuf: make([]byte, 4*0x1000),
	}
	sh.current = sw.newSegment(sh, sk, sl)
	sh.wg.Add(1)
	go func() {
		defer sh.wg.Done()
		sh.loop(sw.cancelCtx)
	}()
	return sh
}
func (sw *segmentsWriter) newSegment(sh *shard, sk shardKey, sl log.Logger) *segment {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	sshard := fmt.Sprintf("%d", sk)
	blockPath := path.Join(pathSegments, sshard, pathAnon, id.String(), pathBlock)
	s := &segment{
		l:         log.With(sl, "segment-id", id.String()),
		ulid:      id,
		heads:     make(map[serviceKey]serviceHead),
		sw:        sw,
		sh:        sh,
		shard:     sk,
		sshard:    sshard,
		blockPath: blockPath,
		doneChan:  make(chan struct{}, 0),
	}
	return s
}

func (s *segment) flush(ctx context.Context) (err error) {
	t1 := time.Now()
	var heads []flushedServiceHead

	defer func() {
		s.cleanup()
		if err != nil {
			s.flushErrMutex.Lock()
			s.flushErr = err
			s.flushErrMutex.Unlock()
		}
		close(s.doneChan)
		s.sw.metrics.flushSegmentDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	}()
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
	err = s.sw.uploadBlock(blockData, s)
	if err != nil {
		return fmt.Errorf("failed to upload block %s: %w", s.ulid.String(), err)
	}
	err = s.sw.storeMeta(ctx, blockMeta, s)
	if err != nil {
		level.Error(s.l).Log("msg", "failed to store meta", "err", err)
		errDLQErr := s.sw.storeMetaDLQ(ctx, blockMeta, s)
		if errDLQErr == nil {
			return nil
		}
		return fmt.Errorf("failed to store meta %s: %w %w", s.ulid.String(), err, errDLQErr)
	}
	return nil
}

func (s *segment) flushBlock(heads []flushedServiceHead) ([]byte, *metastorev1.BlockMeta, error) {
	t1 := time.Now()
	meta := &metastorev1.BlockMeta{
		FormatVersion:   1,
		Id:              s.ulid.String(),
		MinTime:         0,
		MaxTime:         0,
		Shard:           uint32(s.shard),
		CompactionLevel: 0,
		TenantId:        "",
		Datasets:        make([]*metastorev1.Dataset, 0, len(heads)),
		Size:            0,
	}

	blockFile := bytes.NewBuffer(nil)

	w := withWriterOffset(blockFile)

	for i, e := range heads {
		svc, err := concatSegmentHead(e, w)
		if err != nil {
			_ = level.Error(s.l).Log("msg", "failed to concat segment head", "err", err)
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

	meta.Size = uint64(w.offset)
	s.debuginfo.flushBlockDuration = time.Since(t1)
	return blockFile.Bytes(), meta, nil
}

func concatSegmentHead(e flushedServiceHead, w *writerOffset) (*metastorev1.Dataset, error) {
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

	svc := &metastorev1.Dataset{
		TenantId: e.key.tenant,
		Name:     e.key.service,
		MinTime:  e.head.Meta.MinTimeNanos / 1e6,
		MaxTime:  e.head.Meta.MaxTimeNanos/1e6 + 1,
		Size:     uint64(tenantServiceSize),
		//  - 0: profiles.parquet
		//  - 1: index.tsdb
		//  - 2: symbols.symdb
		TableOfContents: offsets,
		ProfileTypes:    ptypes,
	}
	return svc, nil
}

func (s *segment) flushHeads(ctx context.Context) (moved []flushedServiceHead) {
	t1 := time.Now()
	defer func() {
		s.sw.metrics.flushHeadsDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
		s.debuginfo.flushHeadsDuration = time.Since(t1)
	}()
	wg := sync.WaitGroup{}
	mutex := new(sync.Mutex)
	for _, e := range s.heads {
		wg.Add(1)
		e := e
		go func() {
			defer wg.Done()
			eFlushed, err := s.flushHead(ctx, e)

			if err != nil {
				level.Error(s.l).Log("msg", "failed to flush head", "err", err)
			}
			if eFlushed != nil {
				if eFlushed.Meta.NumSamples == 0 {
					_ = level.Debug(s.l).Log("msg", "skipping empty head")
					return
				} else {
					mutex.Lock()
					moved = append(moved, flushedServiceHead{e.key, eFlushed})
					mutex.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	slices.SortFunc(moved, func(i, j flushedServiceHead) int {
		c := strings.Compare(i.key.tenant, j.key.tenant)
		if c != 0 {
			return c
		}
		return strings.Compare(i.key.service, j.key.service)
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
	level.Debug(s.l).Log(
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
	blockPath        string
	doneChan         chan struct{}
	flushErr         error
	flushErrMutex    sync.Mutex
	l                log.Logger

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
	ingest(ctx context.Context, tenantID string, p *profilev1.Profile, id uuid.UUID, labels []*typesv1.LabelPair) error
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
		defer s.flushErrMutex.Unlock()
		res := s.flushErr
		return res
	}
}

func (s *segment) ingest(ctx context.Context, tenantID string, p *profilev1.Profile, id uuid.UUID, labels []*typesv1.LabelPair) error {
	var err error
	k := serviceKey{
		tenant:  tenantID,
		service: phlaremodel.Labels(labels).Get(phlaremodel.LabelNameServiceName),
	}
	s.sw.metrics.segmentIngestBytes.WithLabelValues(s.sshard, tenantID).Observe(float64(p.SizeVT()))
	h, err := s.headForIngest(k)
	if err != nil {
		return err
	}
	return h.Ingest(p, id, labels)
}

func (s *segment) headForIngest(k serviceKey) (*memdb.Head, error) {
	var err error

	s.headsLock.RLock()
	h, ok := s.heads[k]
	s.headsLock.RUnlock()
	if ok {
		return h.head, nil
	}

	s.headsLock.Lock()
	defer s.headsLock.Unlock()
	h, ok = s.heads[k]
	if ok {
		return h.head, nil
	}

	nh, err := memdb.NewHead(s.sw.headMetrics)
	if err != nil {
		return nil, err
	}

	s.heads[k] = serviceHead{
		key:  k,
		head: nh,
	}

	return nh, nil
}

func (s *segment) cleanup() {

}

func (sw *segmentsWriter) uploadBlock(blockData []byte, s *segment) error {
	t1 := time.Now()
	if err := sw.bucket.Upload(context.Background(), s.blockPath, bytes.NewReader(blockData)); err != nil {
		return err
	}
	sw.metrics.segmentBlockSizeBytes.WithLabelValues(s.sshard).Observe(float64(len(blockData)))
	sw.metrics.blockUploadDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	sw.l.Log("msg", "uploaded block", "path", s.blockPath, "time-took", time.Since(t1))

	return nil
}

func (sw *segmentsWriter) storeMeta(ctx context.Context, meta *metastorev1.BlockMeta, s *segment) error {
	t1 := time.Now()

	_, err := sw.metastoreClient.AddBlock(ctx, &metastorev1.AddBlockRequest{
		Block: meta,
	})
	if err != nil {
		sw.metrics.storeMetaErrors.WithLabelValues(s.sshard).Inc()
		return err
	}
	sw.metrics.storeMetaDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	s.debuginfo.storeMetaDuration = time.Since(t1)
	return nil
}

func (sw *segmentsWriter) storeMetaDLQ(ctx context.Context, meta *metastorev1.BlockMeta, s *segment) error {
	metaBlob, err := meta.MarshalVT()
	if err != nil {
		sw.metrics.storeMetaDLQ.WithLabelValues(s.sshard, "err").Inc()
		return err
	}

	fullPath := path.Join(pathDLQ, s.sshard, pathAnon, s.ulid.String(), "meta.pb")
	if err = sw.bucket.Upload(ctx,
		fullPath,
		bytes.NewReader(metaBlob)); err != nil {
		sw.metrics.storeMetaDLQ.WithLabelValues(s.sshard, "err").Inc()
		return err
	}
	sw.metrics.storeMetaDLQ.WithLabelValues(s.sshard, "OK").Inc()
	return nil
}
