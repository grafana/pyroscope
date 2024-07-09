package ingester

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/thanos-io/objstore"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/util/math"
)

const pathSegments = "segments"
const pathAnon = "anon"

// const pathDLQ = "dlq"
const pathBlock = "block.bin"

//const pathMetaPB = "meta.pb" // should we embed it in the block?

type shardKey uint32

type segmentsWriter struct {
	segmentDuration time.Duration
	phlarectx       context.Context
	l               log.Logger
	segments        map[shardKey]*segment
	segmentsLock    sync.RWMutex
	cfg             phlaredb.Config
	limiters        *limiters
	bucket          objstore.Bucket
	metastoreClient *metastoreclient.Client
	wg              sync.WaitGroup
	cancel          context.CancelFunc
}

func newSegmentWriter(phlarectx context.Context, l log.Logger, cfg phlaredb.Config, limiters *limiters, bucket objstore.Bucket, segmentDuration time.Duration, metastoreClient *metastoreclient.Client) *segmentsWriter {
	ctx, cancelFunc := context.WithCancel(context.Background())
	sw := &segmentsWriter{
		segmentDuration: segmentDuration,
		phlarectx:       phlarectx,
		l:               l,
		bucket:          bucket,
		limiters:        limiters,
		cfg:             cfg,
		segments:        make(map[shardKey]*segment),
		metastoreClient: metastoreClient,
		cancel:          cancelFunc,
	}
	sw.wg.Add(1)
	go func() {
		defer sw.wg.Done()
		sw.loop(ctx)
	}()
	return sw
}

//func (sw *segmentsWriter) ingestAndWait(ctx context.Context, shard shardKey, fn func(head segmentIngest) error) error {
//	await, err := sw.ingest(shard, fn)
//	if err != nil {
//		return err
//	}
//	return await.waitFlushed(ctx)
//}

func (sw *segmentsWriter) ingest(shard shardKey, fn func(head segmentIngest) error) (await segmentWaitFlushed, err error) {
	sw.segmentsLock.RLock()
	s, ok := sw.segments[shard]
	if ok {
		s.inFlightProfiles.Add(1)
		defer s.inFlightProfiles.Done()
		sw.segmentsLock.RUnlock()
		return s, fn(s)
	}
	sw.segmentsLock.RUnlock()

	sw.segmentsLock.Lock()
	s, ok = sw.segments[shard]
	if ok {
		s.inFlightProfiles.Add(1)
		defer s.inFlightProfiles.Done()
		sw.segmentsLock.Unlock()
		return s, fn(s)
	}

	s = sw.newSegment(shard)
	sw.segments[shard] = s
	s.inFlightProfiles.Add(1)
	defer s.inFlightProfiles.Done()
	sw.segmentsLock.Unlock()
	return s, fn(s)
}

func (sw *segmentsWriter) Stop() error {
	sw.l.Log("msg", "stopping segments writer")
	sw.cancel()
	sw.wg.Wait()
	sw.l.Log("msg", "segments writer stopped")
	return nil
}

func (sw *segmentsWriter) loop(ctx context.Context) {
	ticker := time.NewTicker(sw.segmentDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sw.flushSegments(context.Background())
		case <-ctx.Done():
			sw.flushSegments(context.Background())
			return
		}
	}
}

func (sw *segmentsWriter) Flush(ctx context.Context) error {
	sw.flushSegments(ctx)
	return nil
}

func (sw *segmentsWriter) flushSegments(ctx context.Context) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "segments flush")
	defer sp.Finish()

	sw.segmentsLock.Lock()
	prev := sw.segments
	sw.segments = make(map[shardKey]*segment)
	sw.segmentsLock.Unlock()

	if len(prev) == 0 {
		return
	}
	_ = level.Debug(sw.l).Log("msg", "writing segments", "count", len(prev))

	var wg sync.WaitGroup
	for _, s := range prev {
		wg.Add(1)
		go func(s *segment) {
			defer wg.Done()
			err := s.flush(ctx)
			if err != nil {
				_ = level.Error(sw.l).Log("msg", "failed to flush segment", "err", err)
			}
		}(s)
	}
	wg.Wait()
}

func (sw *segmentsWriter) newSegment(shard shardKey) *segment {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	dataPath := path.Join(sw.cfg.DataPath, pathSegments, fmt.Sprintf("%d", shard), pathAnon, id.String())
	s := &segment{
		ulid:     id,
		heads:    make(map[serviceKey]serviceHead),
		sw:       sw,
		shard:    shard,
		dataPath: dataPath,
		doneChan: make(chan struct{}),
	}
	_ = level.Debug(sw.l).Log("msg", "new segment", "shard", shard, "segment-id", id.String())
	return s
}

func (s *segment) flush(ctx context.Context) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "segment flush")
	defer sp.Finish()

	_ = level.Debug(s.sw.l).Log("msg", "writing segment block", "shard", s.shard, "segment-id", s.ulid.String())
	defer func() {
		s.cleanup()
		close(s.doneChan)
		_ = level.Debug(s.sw.l).Log("msg", "writing segment block done", "shard", s.shard, "segment-id", s.ulid.String())
	}()

	heads := s.flushHeads(ctx)
	if len(heads) == 0 {
		_ = level.Debug(s.sw.l).Log("msg", "no heads to flush")
		return nil
	}

	blockPath, blockMeta, err := s.flushBlock(ctx, heads)
	if err != nil {
		return err
	}
	err = s.sw.uploadBlock(ctx, blockPath)
	if err != nil {
		return err
	}
	err = s.sw.storeMeta(ctx, blockMeta)
	if err != nil {
		//dlcErr := s.sw.uploadMeta(ctx, blockMeta)
		//if dlcErr != nil {
		//	err = fmt.Errorf("failed to store meta: %w %w", err, fmt.Errorf("failed to upload meta: %w", dlcErr))
		//}
		return err
	}
	return nil
}

func (s *segment) flushBlock(ctx context.Context, heads []serviceHead) (string, *metastorev1.BlockMeta, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "segment flush block")
	defer sp.Finish()
	_ = ctx
	meta := &metastorev1.BlockMeta{
		Id:              s.ulid.String(),
		MinTime:         0,
		MaxTime:         0,
		Shard:           uint32(s.shard),
		CompactionLevel: 0,
		TenantServices:  make([]*metastorev1.TenantService, 0, len(heads)),
	}

	blockPath := path.Join(s.dataPath, pathBlock)
	blockFile, err := os.OpenFile(blockPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return "", nil, err
	}
	defer blockFile.Close()

	w := withWriterOffset(blockFile)

	for i, e := range heads {
		svc, err := concatSegmentHead(e, w)
		if err != nil {
			_ = level.Error(s.sw.l).Log("msg", "failed to concat segment head", "err", err)
			continue
		}
		if i == 0 {
			meta.MinTime = svc.MinTime
			meta.MaxTime = svc.MaxTime
		} else {
			meta.MinTime = math.Min(meta.MinTime, svc.MinTime)
			meta.MaxTime = math.Max(meta.MaxTime, svc.MaxTime)
		}

		meta.TenantServices = append(meta.TenantServices, svc)
	}

	//err = s.dumpMeta(meta)
	//if err != nil {
	//  return err
	//}
	return blockPath, meta, nil
}

//func (s *segment) dumpMeta(meta *metastorev1.BlockMeta) error {
//	bs, err := meta.MarshalVT()
//	if err != nil {
//		return err
//	}
//	err = os.WriteFile(path.Join(s.dataPath, pathMetaPB), bs, 0644)
//	if err != nil {
//		return err
//	}
//	bs, err = json.Marshal(meta)
//	err = os.WriteFile(path.Join(s.dataPath, pathMetaJson), []byte(bs), 0644)
//	if err != nil {
//		return err
//	}
//	return nil
//}

func concatSegmentHead(e serviceHead, w *writerOffset) (*metastorev1.TenantService, error) {
	tenantServiceOffset := w.offset
	b := e.head.Meta()
	profiles, index, symbols := getFilesForSegment(b)

	offsets, err := concatFiles(w, e.head, profiles, index, symbols)
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
	}
	return svc, nil
}

func (s *segment) flushHeads(ctx context.Context) []serviceHead {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "segment flush heads")
	defer sp.Finish()
	s.inFlightProfiles.Wait()
	moved := make([]serviceHead, 0, len(s.heads))
	for _, e := range s.heads {
		if err := e.head.Flush(ctx); err != nil {
			_ = level.Error(s.sw.l).Log("msg", "failed to flush head", "err", err, "head", e.head.BlockID())
			continue
		}
		if err := e.head.Move(); err != nil {
			_ = level.Error(s.sw.l).Log("msg", "failed to move head", "err", err, "head", e.head.BlockID())
			continue
		}
		profiles, index, symbols := getFilesForSegment(e.head.Meta())
		if profiles == nil || index == nil || symbols == nil {
			_ = level.Error(s.sw.l).Log("msg", "failed to find files", "head", e.head.BlockID())
			continue
		}
		if e.head.GetMetaStats().NumSamples == 0 {
			_ = level.Debug(s.sw.l).Log("msg", "skipping empty head", "head", e.head.BlockID())
			continue
		}
		moved = append(moved, e)
	}
	slices.SortFunc(moved, func(i, j serviceHead) int {
		c := strings.Compare(i.key.tenant, j.key.tenant)
		if c != 0 {
			return c
		}
		return strings.Compare(i.key.service, j.key.service)
	})
	return moved
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
	inFlightProfiles sync.WaitGroup
	heads            map[serviceKey]serviceHead
	headsLock        sync.RWMutex
	sw               *segmentsWriter
	dataPath         string
	doneChan         chan struct{}
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
		return ctx.Err()
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

	l := s.sw.limiters.get(k.tenant)
	cfg := s.sw.cfg
	cfg.DataPath = path.Join(s.dataPath, s.ulid.String())
	cfg.SymDBFormat = symdb.FormatV3

	nh, err := phlaredb.NewHead(s.sw.phlarectx, cfg, l)
	if err != nil {
		return nil, err
	}
	_ = level.Debug(s.sw.l).Log("msg", "new head", "head-id", nh.BlockID(), "segment-id", s.ulid.String(), "tenant", k.tenant, "service", k.service)

	s.heads[k] = serviceHead{
		key:  k,
		head: nh,
	}

	return nh, nil
}

func (s *segment) cleanup() {
	if err := os.RemoveAll(s.dataPath); err != nil {
		_ = level.Error(s.sw.l).Log("msg", "failed to cleanup segment", "err", err, "f", s.dataPath)
	}
}

func (sw *segmentsWriter) uploadBlock(ctx context.Context, blockPath string) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "segment uploadBlock")
	defer sp.Finish()
	_ = ctx

	dst, err := filepath.Rel(sw.cfg.DataPath, blockPath)
	if err != nil {
		return err
	}
	if err := objstore.UploadFile(sw.phlarectx, sw.l, sw.bucket, blockPath, dst); err != nil {
		return err
	}
	sw.l.Log("msg", "uploaded block", "path", dst)
	return nil
}

//func (sw *segmentsWriter) uploadMeta(ctx context.Context, meta *metastorev1.BlockMeta) error {
//	sp, ctx := opentracing.StartSpanFromContext(ctx, "segment uploadMeta")
//	defer sp.Finish()
//	data, err := meta.MarshalVT()
//	if err != nil {
//		return err
//	}
//	//dlc/{shard}/{tenant}/{block_id}/meta.pb
//	dst := fmt.Sprintf("%s/%d/%s/%s/meta.pb", pathDLQ, meta.Shard, pathAnon, meta.Id)
//	err = sw.bucket.Upload(ctx, dst, bytes.NewReader(data))
//	if err != nil {
//		return err
//	}
//	sw.l.Log("msg", "uploaded meta", "segment-id", meta.Id, "shard", meta.Shard)
//	return nil
//}

func (sw *segmentsWriter) storeMeta(ctx context.Context, meta *metastorev1.BlockMeta) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "segment store meta")
	defer sp.Finish()

	resp, err := sw.metastoreClient.AddBlock(ctx, &metastorev1.AddBlockRequest{
		Block: meta,
	})
	if err != nil {
		return err
	}
	_ = resp
	//var tenants []string
	//for _, svc := range meta.TenantServices {
	//	tenants = append(tenants, svc.TenantId)
	//}
	//blocks, err := sw.metastoreclient.ListBlocksForQuery(ctx, &metastorev1.ListBlocksForQueryRequest{
	//	TenantId:  tenants,
	//	StartTime: time.Now().Add(-time.Hour).UnixMilli(),
	//	EndTime:   time.Now().UnixMilli(),
	//	Query:     "{}",
	//})
	//if err != nil {
	//	return nil
	//}
	//_ = blocks

	return nil
}

func getFilesForSegment(b *block.Meta) (profiles *block.File, index *block.File, symbols *block.File) {
	profiles = b.FileByRelPath("profiles.parquet")
	index = b.FileByRelPath("index.tsdb")
	symbols = b.FileByRelPath("symbols.symdb")
	return
}
