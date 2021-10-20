package storage

import (
	"context"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v2"

	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

func (s *Storage) Reclaim(ctx context.Context, rp *segment.RetentionPolicy) error {
	if rp.SizeLimit() == 0 {
		return nil
	}

	segmentKeys, err := s.segmentKeys()
	if err != nil || len(segmentKeys) == 0 {
		return err
	}

	var (
		// Occupied disk space.
		used = dirSize(s.config.StoragePath)
		// Size in bytes to be reclaimed.
		rs = rp.CapacityToReclaim(used, 0.05).Bytes()
		// Size in bytes to be reclaimed per segment, if applicable.
		rsps = rs / len(segmentKeys)
		// Counters for deleted data.
		stats reclaimStats
	)

	for k := range segmentKeys {
		r := &reclaim{
			stats: &stats,
			sks:   k,
			size:  rsps,
		}
		if err = s.deleteDataBefore(ctx, r); err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) DeleteDataBefore(ctx context.Context, rp *segment.RetentionPolicy) error {
	if rp.LowerTimeBoundary() == zeroTime {
		return nil
	}

	segmentKeys, err := s.segmentKeys()
	if err != nil || len(segmentKeys) == 0 {
		return err
	}

	var stats reclaimStats
	for k, v := range segmentKeys {
		r := &reclaim{
			stats: &stats,
			rp:    rp,
			sks:   k,
			sk:    v,
		}
		if err = s.deleteDataBefore(ctx, r); err != nil {
			return err
		}
	}

	return nil
}

type reclaim struct {
	stats *reclaimStats
	rp    *segment.RetentionPolicy
	sk    *segment.Key
	nodes []segmentNode // Segment nodes to delete.
	sks   string        // Segment key string.
	size  int           // Capacity to reclaim for the segment.
}

type reclaimStats struct {
	removedTrees            int
	estimatedReclaimedSpace int
}

type segmentNode struct {
	depth int
	time  int64
}

const batchSize = 1024

func (s *Storage) reclaimSpace(ctx context.Context, r *reclaim) (err error) {
	var (
		batch     = s.dbTrees.NewWriteBatch()
		removed   int
		reclaimed int
	)
	defer func() {
		err = batch.Flush()
		r.stats.removedTrees += removed
		r.stats.estimatedReclaimedSpace += reclaimed
	}()

	return s.dbTrees.View(func(txn *badger.Txn) error {
		// Lower-level trees go first because of the lexicographical order.
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			// The prefix matches all trees of the segment.
			Prefix: []byte("t:" + r.sks),
			// We count all version so that our estimation is more precise
			// but slightly higher than the actual size in practice,
			// meaning that we delete less data (and reclaim less space).
			AllVersions: true,
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if r.size-reclaimed <= 0 {
				return nil
			}

			item := it.Item()
			// A copy must be taken. The slice is reused by iterator but
			// also put to the batch.
			tk := item.KeyCopy(nil)
			// A sanity check – a tree key can not be shorter 18 bytes:
			// t:<segment_key>{}:0:1234567890
			if len(tk) <= 18 {
				continue
			}

			switch err = batch.Delete(tk); {
			case err == nil:
			case errors.Is(err, badger.ErrKeyNotFound):
				continue
			default:
				return err
			}

			reclaimed += int(item.EstimatedSize())
			if removed++; removed%batchSize != 0 {
				continue
			}

			s.trees.Discard(string(tk[2:]))
			// TODO(kolesnikovae): remove corresponding segment nodes

			// Once per batch check if the context has been cancelled.
			// If that's the case, the batch is to be flushed in defer
			// section. Otherwise, flush it and create a new batch.
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = batch.Flush(); err != nil {
				return err
			}
			batch = s.dbTrees.NewWriteBatch()
		}

		return nil
	})
}

// Instead of removing every tree in an individual transaction for each
// of them, blocking the segment for a long time, we remember them and
// drop in batches after the segment is released.
//
// There is a better way of removing these trees: we could calculate
// non-overlapping prefixes for depth levels, e.g:
//     0:163474....
//     1:1634745...
//     2:16347456..
//
// And drop the data (both in cache and on disk) using these prefixes via
// db.DropPrefix and cache.DiscardPrefix. Remaining trees (with overlapping
// prefixes) would be removed in batches. Only after successful commit,
// segment nodes can be safely removed.
//
// That would be especially efficient in cases when data removed for a very
// long period. For example, when retention-period is enabled for the first
// time on a server with historical data.
func (s *Storage) deleteDataBefore(ctx context.Context, r *reclaim) (err error) {
	cached, ok := s.segments.Lookup(r.sks)
	if !ok {
		return nil
	}

	// To avoid a potential inconsistency when DeleteNodesBefore
	// fails in the process, trees should be removed first.
	seg := cached.(*segment.Segment)
	r.nodes = r.nodes[:0]
	deleted, err := seg.WalkNodesToDelete(r.rp, func(d int, t time.Time) error {
		r.nodes = append(r.nodes, segmentNode{d, t.Unix()})
		return nil
	})
	if err != nil {
		return err
	}
	if deleted {
		return s.deleteSegmentAndRelatedData(r.sk)
	}

	batch := s.dbTrees.NewWriteBatch()
	defer batch.Cancel()

	for _, n := range r.nodes {
		treeKey := segment.TreeKey(r.sks, n.depth, n.time)
		switch err = batch.Delete([]byte("t:" + treeKey)); {
		case err == nil:
		case errors.Is(err, badger.ErrKeyNotFound):
			continue
		default:
			return err
		}
		if r.stats.removedTrees++; r.stats.removedTrees%batchSize != 0 {
			// Once per batch check if the context has been cancelled.
			// If that's the case, the batch is to be flushed in defer
			// section. Otherwise, flush it and create a new batch.
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = batch.Flush(); err != nil {
				return err
			}
			batch = s.dbTrees.NewWriteBatch()
		}
	}

	_, err = seg.DeleteNodesBefore(r.rp)
	return err
}

// segmentKeys returns a map of valid segment keys found in the storage.
// It returns both parsed, and string representation.
func (s *Storage) segmentKeys() (map[string]*segment.Key, error) {
	segmentKeys := make(map[string]*segment.Key)
	return segmentKeys, s.dbSegments.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			Prefix:         []byte("s:"),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if k := it.Item().Key(); len(k) > 2 {
				// k must not be reused outside of the transaction.
				key, err := segment.ParseKey(string(k[2:]))
				if err != nil {
					continue
				}
				segmentKeys[key.Normalized()] = key
			}
		}
		return nil
	})
}
