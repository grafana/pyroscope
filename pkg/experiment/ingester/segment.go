package ingester

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/pkg/experiment/ingester/loki/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/client"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util/math"
)

const pathSegments = "segments"
const pathAnon = tenant.DefaultTenantID
const pathBlock = "block.bin"

type shardKey uint32

type segmentsWriter struct {
	segmentDuration time.Duration
	phlarectx       context.Context
	l               log.Logger
	shards          map[shardKey]*shard
	shardsLock      sync.RWMutex
	cfg             phlaredb.Config
	bucket          objstore.Bucket
	metastoreClient *metastoreclient.Client
	cancel          context.CancelFunc
	metrics         *segmentMetrics
	headMetrics     *phlaredb.HeadMetrics
	cancelCtx       context.Context
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

func newSegmentWriter(phlarectx context.Context, l log.Logger, metrics *segmentMetrics, hm *phlaredb.HeadMetrics, cfg phlaredb.Config, bucket objstore.Bucket, segmentDuration time.Duration, metastoreClient *metastoreclient.Client) *segmentsWriter {
	ctx, cancelFunc := context.WithCancel(context.Background())
	sw := &segmentsWriter{
		metrics:         metrics,
		headMetrics:     hm,
		segmentDuration: segmentDuration,
		phlarectx:       phlarectx,
		l:               l,
		bucket:          bucket,
		cfg:             cfg,
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
	dataPath := path.Join(sw.cfg.DataPath, pathSegments, fmt.Sprintf("%d", sk), pathAnon, id.String())
	s := &segment{
		l:        log.With(sl, "segment-id", id.String()),
		ulid:     id,
		heads:    make(map[serviceKey]serviceHead),
		sw:       sw,
		sh:       sh,
		shard:    sk,
		sshard:   fmt.Sprintf("%d", sk),
		dataPath: dataPath,
		doneChan: make(chan struct{}),
	}
	return s
}

func (s *segment) flush(ctx context.Context) error {
	t1 := time.Now()
	var heads []serviceHead

	defer func() {
		s.cleanup()
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

	blockPath, blockMeta, err := s.flushBlock(heads)
	if err != nil {
		return fmt.Errorf("failed to flush block %s: %w", s.ulid.String(), err)
	}
	err = s.sw.uploadBlock(blockPath, s)
	if err != nil {
		return fmt.Errorf("failed to upload block %s: %w", s.ulid.String(), err)
	}
	err = s.sw.storeMeta(ctx, blockMeta, s)
	if err != nil {
		return fmt.Errorf("failed to store meta %s: %w", s.ulid.String(), err)
	}
	return nil
}

func (s *segment) flushBlock(heads []serviceHead) (string, *metastorev1.BlockMeta, error) {
	t1 := time.Now()
	meta := &metastorev1.BlockMeta{
		FormatVersion:   1,
		Id:              s.ulid.String(),
		MinTime:         0,
		MaxTime:         0,
		Shard:           uint32(s.shard),
		CompactionLevel: 0,
		TenantId:        "",
		TenantServices:  make([]*metastorev1.TenantService, 0, len(heads)),
		Size:            0,
	}

	blockPath := path.Join(s.dataPath, pathBlock)
	blockFile, err := os.OpenFile(blockPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return "", nil, err
	}
	defer blockFile.Close()

	w := withWriterOffset(blockFile)

	for i, e := range heads {
		svc, err := concatSegmentHead(s.sh, e, w)
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
		meta.TenantServices = append(meta.TenantServices, svc)
	}

	meta.Size = uint64(w.offset)
	s.debuginfo.flushBlockDuration = time.Since(t1)
	return blockPath, meta, nil
}

func concatSegmentHead(sh *shard, e serviceHead, w *writerOffset) (*metastorev1.TenantService, error) {
	tenantServiceOffset := w.offset
	b := e.head.Meta()
	ptypes := e.head.MustProfileTypeNames()

	profiles, x, symbols := getFilesForSegment(e.head, b)
	defer index.PutBufferWriterToPool(x)

	offsets := make([]uint64, 3)
	var err error
	offsets[0], err = concatFile(w, e.head, profiles, sh.concatBuf)
	if err != nil {
		return nil, err
	}
	offsets[1] = uint64(w.offset)
	indexBytes, _, _ := x.Buffer()
	_, err = w.Write(indexBytes)
	if err != nil {
		return nil, err
	}
	offsets[2], err = concatFile(w, e.head, symbols, sh.concatBuf)
	if err != nil {
		return nil, err
	}

	tenantServiceSize := w.offset - tenantServiceOffset

	svc := &metastorev1.TenantService{
		TenantId: e.key.tenant,
		Name:     e.key.service,
		MinTime:  int64(b.MinTime),
		MaxTime:  int64(b.MaxTime),
		Size:     uint64(tenantServiceSize),
		//  - 0: profiles.parquet
		//  - 1: index.tsdb
		//  - 2: symbols.symdb
		TableOfContents: offsets,
		ProfileTypes:    ptypes,
	}
	return svc, nil
}

func (s *segment) flushHeads(ctx context.Context) (moved []serviceHead) {
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
			eMoved, err := s.flushHead(ctx, e)
			if err != nil {
				level.Error(s.l).Log("msg", "failed to flush head", "err", err)
			}
			if eMoved {
				mutex.Lock()
				moved = append(moved, e)
				mutex.Unlock()
			}
		}()
	}
	wg.Wait()

	slices.SortFunc(moved, func(i, j serviceHead) int {
		c := strings.Compare(i.key.tenant, j.key.tenant)
		if c != 0 {
			return c
		}
		return strings.Compare(i.key.service, j.key.service)
	})
	return moved
}

func (s *segment) flushHead(ctx context.Context, e serviceHead) (moved bool, err error) {
	th := time.Now()
	if err := e.head.Flush(ctx); err != nil {
		s.sw.metrics.flushServiceHeadDuration.WithLabelValues(s.sshard, e.key.tenant).Observe(time.Since(th).Seconds())
		s.sw.metrics.flushServiceHeadError.WithLabelValues(s.sshard, e.key.tenant).Inc()
		return false, fmt.Errorf("failed to flush head %v: %w", e.head.BlockID(), err)
	}
	s.sw.metrics.flushServiceHeadDuration.WithLabelValues(s.sshard, e.key.tenant).Observe(time.Since(th).Seconds())
	stats, _ := json.Marshal(e.head.GetMetaStats())
	level.Debug(s.l).Log(
		"msg", "flushed head",
		"head", e.head.BlockID(),
		"stats", stats,
		"head-flush-duration", time.Since(th).String(),
	)
	if err := e.head.Move(); err != nil {
		if e.head.GetMetaStats().NumSamples == 0 {
			_ = level.Debug(s.l).Log("msg", "skipping empty head", "head", e.head.BlockID())
			return false, nil
		}
		s.sw.metrics.flushServiceHeadError.WithLabelValues(s.sshard, e.key.tenant).Inc()
		return false, fmt.Errorf("failed to move head %v: %w", e.head.BlockID(), err)
	}
	profiles, index, symbols := getFilesForSegment(e.head, e.head.Meta())
	if profiles == nil || index == nil || symbols == nil {
		s.sw.metrics.flushServiceHeadError.WithLabelValues(s.sshard, e.key.tenant).Inc()
		return false, fmt.Errorf("failed to find files %v %v %v", profiles, index, symbols)
	}
	return true, nil
}

type serviceKey struct {
	tenant  string
	service string
}
type serviceHead struct {
	key  serviceKey
	head *phlaredb.Head
}

type segment struct {
	ulid             ulid.ULID
	shard            shardKey
	sshard           string
	inFlightProfiles sync.WaitGroup
	heads            map[serviceKey]serviceHead
	headsLock        sync.RWMutex
	sw               *segmentsWriter
	dataPath         string
	doneChan         chan struct{}
	l                log.Logger

	debuginfo struct {
		movedHeads         int
		waitInflight       time.Duration
		flushHeadsDuration time.Duration
		flushBlockDuration time.Duration
		storeMetaDuration  time.Duration
	}
	sh *shard
}

type segmentIngest interface {
	ingest(ctx context.Context, tenantID string, p *profilev1.Profile, id uuid.UUID, labels ...*typesv1.LabelPair) error
}

type segmentWaitFlushed interface {
	waitFlushed(ctx context.Context) error
}

func (s *segment) waitFlushed(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("waitFlushed: %s %w", s.ulid.String(), ctx.Err())
	case <-s.doneChan:
		return nil
	}
}

func (s *segment) ingest(ctx context.Context, tenantID string, p *profilev1.Profile, id uuid.UUID, labels ...*typesv1.LabelPair) error {
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
	return h.Ingest(ctx, p, id, labels...)
}

func (s *segment) headForIngest(k serviceKey) (*phlaredb.Head, error) {
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

	cfg := s.sw.cfg
	cfg.DataPath = path.Join(s.dataPath)
	cfg.SymDBFormat = symdb.FormatV3

	nh, err := phlaredb.NewHead(s.sw.phlarectx, cfg, s.sw.headMetrics, noopLimiter{})
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
	if err := os.RemoveAll(s.dataPath); err != nil {
		_ = level.Error(s.l).Log("msg", "failed to cleanup segment", "err", err, "f", s.dataPath)
	}
}

func (sw *segmentsWriter) uploadBlock(blockPath string, s *segment) error {
	t1 := time.Now()

	dst, err := filepath.Rel(sw.cfg.DataPath, blockPath)
	if err != nil {
		return err
	}
	if err := objstore.UploadFile(sw.phlarectx, sw.l, sw.bucket, blockPath, dst); err != nil {
		return err
	}
	st, _ := os.Stat(blockPath)
	if st != nil {
		sw.metrics.segmentBlockSizeBytes.WithLabelValues(s.sshard).Observe(float64(st.Size()))
	}
	sw.metrics.blockUploadDuration.WithLabelValues(s.sshard).Observe(time.Since(t1).Seconds())
	sw.l.Log("msg", "uploaded block", "path", dst, "time-took", time.Since(t1))

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

func getFilesForSegment(_ *phlaredb.Head, b *block.Meta) (profiles *block.File, index *index.BufferWriter, symbols *block.File) {
	profiles = b.FileByRelPath("profiles.parquet")
	// FIXME
	// index = head.TSDBIndex()
	symbols = b.FileByRelPath("symbols.symdb")
	return
}

type noopLimiter struct{}

func (noopLimiter) AllowProfile(model.Fingerprint, phlaremodel.Labels, int64) error { return nil }

func (noopLimiter) Stop() {}
