package storegateway

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/mimir/pkg/storegateway"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	phlareobj "github.com/grafana/phlare/pkg/objstore"
	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/phlaredb/block"
)

// TODO move this to a config.
const blockSyncConcurrency = 100

type BucketStore struct {
	bucket            phlareobj.Bucket
	tenantID, syncDir string

	logger log.Logger

	blocksMx sync.RWMutex
	blocks   map[ulid.ULID]*Block
	blockSet *bucketBlockSet

	filters []BlockMetaFilter
	metrics *Metrics
	stats   storegateway.BucketStoreStats
}

func NewBucketStore(bucket phlareobj.Bucket, tenantID string, syncDir string, filters []BlockMetaFilter, logger log.Logger, Metrics *Metrics) (*BucketStore, error) {
	s := &BucketStore{
		bucket:   phlareobj.NewPrefixedBucket(bucket, tenantID+"/phlaredb"),
		tenantID: tenantID,
		syncDir:  syncDir,
		logger:   logger,
		filters:  filters,
		blockSet: newBucketBlockSet(),
		blocks:   map[ulid.ULID]*Block{},
		metrics:  Metrics,
	}

	if err := os.MkdirAll(syncDir, 0o750); err != nil {
		return nil, errors.Wrap(err, "create dir")
	}

	return s, nil
}

func (b *BucketStore) InitialSync(ctx context.Context) error {
	if err := b.SyncBlocks(ctx); err != nil {
		return errors.Wrap(err, "sync block")
	}

	fis, err := os.ReadDir(b.syncDir)
	if err != nil {
		return errors.Wrap(err, "read dir")
	}
	names := make([]string, 0, len(fis))
	for _, fi := range fis {
		names = append(names, fi.Name())
	}
	for _, n := range names {
		id, ok := block.IsBlockDir(n)
		if !ok {
			continue
		}
		if b := b.getBlock(id); b != nil {
			continue
		}

		// No such block loaded, remove the local dir.
		if err := os.RemoveAll(path.Join(b.syncDir, id.String())); err != nil {
			level.Warn(b.logger).Log("msg", "failed to remove block which is not needed", "err", err)
		}
	}
	return nil
}

func (s *BucketStore) getBlock(id ulid.ULID) *Block {
	s.blocksMx.RLock()
	defer s.blocksMx.RUnlock()
	return s.blocks[id]
}

func (s *BucketStore) SyncBlocks(ctx context.Context) error {
	metas, metaFetchErr := s.fetchBlocksMeta(ctx)
	// For partial view allow adding new blocks at least.
	if metaFetchErr != nil && metas == nil {
		return metaFetchErr
	}

	var wg sync.WaitGroup
	blockc := make(chan *block.Meta)

	for i := 0; i < blockSyncConcurrency; i++ {
		wg.Add(1)
		go func() {
			for meta := range blockc {
				if err := s.addBlock(ctx, meta); err != nil {
					continue
				}
			}
			wg.Done()
		}()
	}

	for id, meta := range metas {
		if b := s.getBlock(id); b != nil {
			continue
		}
		select {
		case <-ctx.Done():
		case blockc <- meta:
		}
	}

	close(blockc)
	wg.Wait()

	if metaFetchErr != nil {
		return metaFetchErr
	}

	// Drop all blocks that are no longer present in the bucket.
	for id := range s.blocks {
		if _, ok := metas[id]; ok {
			continue
		}
		if err := s.removeBlock(id); err != nil {
			level.Warn(s.logger).Log("msg", "drop of outdated block failed", "block", id, "err", err)
		}
		level.Info(s.logger).Log("msg", "dropped outdated block", "block", id)
	}
	s.stats.BlocksLoaded = len(s.blocks)

	return nil
}

func (bs *BucketStore) addBlock(ctx context.Context, meta *block.Meta) (err error) {
	level.Debug(bs.logger).Log("msg", "loading new block", "id", meta.ULID)

	dir := bs.localPath(meta.ULID.String())
	start := time.Now()
	defer func() {
		if err != nil {
			bs.metrics.blockLoadFailures.Inc()
			if err2 := os.RemoveAll(dir); err2 != nil {
				level.Warn(bs.logger).Log("msg", "failed to remove block we cannot load", "err", err2)
			}
			level.Warn(bs.logger).Log("msg", "loading block failed", "elapsed", time.Since(start), "id", meta.ULID, "err", err)
		} else {
			level.Info(bs.logger).Log("msg", "loaded new block", "elapsed", time.Since(start), "id", meta.ULID)
		}
	}()

	bs.metrics.blockLoads.Inc()

	b, err := func() (*Block, error) {
		bs.blocksMx.Lock()
		defer bs.blocksMx.Unlock()
		b, err := bs.createBlock(ctx, meta)
		if err != nil {
			return nil, errors.Wrap(err, "load block from disk")
		}
		bs.blockSet.add(b)
		bs.blocks[meta.ULID] = b
		return b, nil
	}()
	if err != nil {
		return err
	}
	// Load the block into memory if it's within the last 24 hours.
	// Todo make this configurable
	if phlaredb.InRange(b, model.Now().Add(-24*time.Hour), model.Now()) {
		level.Debug(bs.logger).Log("msg", "opening block",
			"id", meta.ULID.String(),
			"min", b.meta.MinTime.Time().Format(time.RFC3339),
			"max", b.meta.MaxTime.Time().Format(time.RFC3339),
		)

		start := time.Now()
		defer func() {
			level.Info(bs.logger).Log("msg", "block opened", "duration", time.Since(start), "id", meta.ULID.String())
		}()
		if err := b.Open(ctx); err != nil {
			level.Error(bs.logger).Log("msg", "open block", "err", err)
		}
	}
	return nil
}

func (b *BucketStore) Stats() storegateway.BucketStoreStats {
	return b.stats
}

func (s *BucketStore) removeBlock(id ulid.ULID) (returnErr error) {
	defer func() {
		if returnErr != nil {
			s.metrics.blockDropFailures.Inc()
		}
	}()

	s.blocksMx.Lock()
	b, ok := s.blocks[id]
	if ok {
		s.blockSet.remove(id)
		delete(s.blocks, id)
	}
	s.blocksMx.Unlock()

	if !ok {
		return nil
	}

	// // The block has already been removed from BucketStore, so we track it as removed
	// // even if releasing its resources could fail below.
	s.metrics.blockDrops.Inc()

	if err := b.Close(); err != nil {
		return errors.Wrap(err, "close block")
	}
	if err := os.RemoveAll(s.localPath(id.String())); err != nil {
		return errors.Wrap(err, "delete block")
	}
	return nil
}

func (s *BucketStore) localPath(id string) string {
	return filepath.Join(s.syncDir, id)
}

// RemoveBlocksAndClose remove all blocks from local disk and releases all resources associated with the BucketStore.
func (s *BucketStore) RemoveBlocksAndClose() error {
	if err := os.RemoveAll(s.syncDir); err != nil {
		return errors.Wrap(err, "delete block")
	}
	return nil
}

func (s *BucketStore) fetchBlocksMeta(ctx context.Context) (map[ulid.ULID]*block.Meta, error) {
	var (
		to   = time.Now()
		from = to.Add(-time.Hour * 24 * 31) // todo make this configurable
	)

	var (
		metas []*block.Meta
		mtx   sync.Mutex
	)

	start := time.Now()
	level.Debug(s.logger).Log("msg", "fetching blocks meta", "from", from, "to", to)
	defer func() {
		level.Debug(s.logger).Log("msg", "fetched blocks meta", "total", len(metas), "elapsed", time.Since(start))
	}()
	if err := block.IterBlockMetas(ctx, s.bucket, from, to, func(m *block.Meta) {
		mtx.Lock()
		defer mtx.Unlock()
		metas = append(metas, m)
	}); err != nil {
		return nil, errors.Wrap(err, "iter block metas")
	}

	metaMap := lo.SliceToMap(metas, func(item *block.Meta) (ulid.ULID, *block.Meta) {
		return item.ULID, item
	})
	if len(metaMap) == 0 {
		return nil, nil
	}
	for _, filter := range s.filters {
		// NOTE: filter can update synced metric accordingly to the reason of the exclude.
		// todo: wire up the filter with the metrics.
		if err := filter.Filter(ctx, metaMap, s.metrics.Synced); err != nil {
			return nil, errors.Wrap(err, "filter metas")
		}
	}
	return metaMap, nil
}

// bucketBlockSet holds all blocks.
type bucketBlockSet struct {
	mtx    sync.RWMutex
	blocks []*Block // Blocks sorted by mint, then maxt.
}

// newBucketBlockSet initializes a new set with the known downsampling windows hard-configured.
// (Mimir only supports no-downsampling)
// The set currently does not support arbitrary ranges.
func newBucketBlockSet() *bucketBlockSet {
	return &bucketBlockSet{}
}

func (s *bucketBlockSet) add(b *Block) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.blocks = append(s.blocks, b)

	// Always sort blocks by min time, then max time.
	sort.Slice(s.blocks, func(j, k int) bool {
		if s.blocks[j].meta.MinTime == s.blocks[k].meta.MinTime {
			return s.blocks[j].meta.MaxTime < s.blocks[k].meta.MaxTime
		}
		return s.blocks[j].meta.MinTime < s.blocks[k].meta.MinTime
	})
}

func (s *bucketBlockSet) remove(id ulid.ULID) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for i, b := range s.blocks {
		if b.meta.ULID != id {
			continue
		}
		s.blocks = append(s.blocks[:i], s.blocks[i+1:]...)
		return
	}
}

// getFor returns a time-ordered list of blocks that cover date between mint and maxt.
// It supports overlapping blocks.
//
// NOTE: s.blocks are expected to be sorted in minTime order.
func (s *bucketBlockSet) getFor(mint, maxt model.Time) (bs []*Block) {
	if mint > maxt {
		return nil
	}

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	// Fill the given interval with the blocks within the request mint and maxt.
	for _, b := range s.blocks {
		if b.meta.MaxTime <= mint {
			continue
		}
		// NOTE: Block intervals are half-open: [b.MinTime, b.MaxTime).
		if b.meta.MinTime > maxt {
			break
		}

		bs = append(bs, b)
	}

	return bs
}
