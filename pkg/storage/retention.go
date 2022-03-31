package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v2"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

func (s *Storage) EnforceRetentionPolicy(rp *segment.RetentionPolicy) error {
	s.logger.Debug("enforcing retention policy")
	if !rp.AbsoluteTime.IsZero() {
		// It may make sense running it concurrently with some throttling.
		err := s.iterateOverAllSegments(func(k *segment.Key) error {
			return s.deleteSegmentData(k, rp)
		})
		if errors.Is(err, errClosed) {
			s.logger.Info("enforcing canceled")
			return nil
		}
		return err
	}
	if !rp.ExemplarsRetentionTime.IsZero() {
		if err := s.profiles.truncateBefore(context.TODO(), rp.ExemplarsRetentionTime); err != nil {
			return fmt.Errorf("failed to truncate profiles storage: %w", err)
		}
	}
	return nil
}

func (s *Storage) deleteSegmentData(k *segment.Key, rp *segment.RetentionPolicy) error {
	sk := k.SegmentKey()
	cached, ok := s.segments.Lookup(sk)
	if !ok {
		return nil
	}

	// Instead of removing every tree in an individual transaction for each
	// of them, blocking the segment for a long time, we remember them and
	// drop in batches after the segment is released.
	//
	// To avoid a potential inconsistency when DeleteNodesBefore fails in the
	// process, trees should be removed first. Only after successful commit
	// segment nodes can be safely removed to guaranty idempotency.

	// TODO(kolesnikovae):
	//  There is a better way of removing these trees: we could calculate
	//  non-overlapping prefixes for depth levels, and drop the data
	//  (both in cache and on disk) using these prefixes.
	//  Remaining trees (with overlapping prefixes) would be removed
	//  in batches. That would be especially efficient in cases when data
	//  removed for a very long period. For example, when retention-period
	//  is enabled for the first time on a server with historical data.

	type segmentNode struct {
		depth int
		time  int64
	}

	nodes := make([]segmentNode, 0)
	seg := cached.(*segment.Segment)
	deleted, err := seg.WalkNodesToDelete(rp, func(d int, t time.Time) error {
		nodes = append(nodes, segmentNode{d, t.Unix()})
		return nil
	})
	if err != nil {
		return err
	}
	if deleted {
		return s.deleteSegmentAndRelatedData(k)
	}

	var removed int64
	batchSize := s.trees.MaxBatchCount()
	batch := s.trees.NewWriteBatch()
	defer func() {
		batch.Cancel()
	}()

	for _, n := range nodes {
		treeKey := segment.TreeKey(sk, n.depth, n.time)
		s.trees.Discard(treeKey)
		switch err = batch.Delete(treePrefix.key(treeKey)); {
		case err == nil:
		case errors.Is(err, badger.ErrKeyNotFound):
			continue
		default:
			return err
		}
		// It is not possible to make size estimation without reading
		// the item. Therefore, the call does not report reclaimed space.
		if removed++; removed%batchSize == 0 {
			if batch, err = s.flushTreeBatch(batch); err != nil {
				return err
			}
		}
	}

	// Flush remaining items, if any: it's important to make sure
	// all trees were removed before deleting segment nodes - see
	// note on a potential inconsistency above.
	if removed%batchSize != 0 {
		if err = batch.Flush(); err != nil {
			return err
		}
	}

	_, err = seg.DeleteNodesBefore(rp)
	return err
}

// reclaimSegmentSpace is aimed to reclaim specified size by removing
// trees for the given segment. The amount of deleted trees is determined
// based on the KV item size estimation.
//
// Unfortunately, due to the fact that badger DB reclaims disk space
// eventually, there is no way to juxtapose the actual occupied disk size
// and the number of items to remove based on their estimated size.
func (s *Storage) reclaimSegmentSpace(k *segment.Key, size int64) error {
	batchSize := s.trees.MaxBatchCount()
	batch := s.trees.NewWriteBatch()
	defer func() {
		batch.Cancel()
	}()

	var (
		removed   int64
		reclaimed int64
		err       error
	)

	// Keep track of the most recent removed tree time per every segment level.
	rp := &segment.RetentionPolicy{Levels: make(map[int]time.Time)}
	err = s.trees.View(func(txn *badger.Txn) error {
		// Lower-level trees come first because of the lexicographical order:
		// from the very first tree to the most recent one, from the lowest
		// level (with highest resolution) to the highest.
		it := txn.NewIterator(badger.IteratorOptions{
			// We count all version so that our estimation is more precise
			// but slightly higher than the actual size in practice,
			// meaning that we delete less data (and reclaim less space);
			// otherwise there is a chance to remove more trees than needed.
			AllVersions: true,
			// The prefix matches all trees in the segment.
			Prefix: treePrefix.key(k.SegmentKey()),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if size-reclaimed <= 0 {
				return nil
			}

			item := it.Item()
			if tk, ok := treePrefix.trim(item.Key()); ok {
				treeKey := string(tk)
				s.trees.Discard(treeKey)
				// Update the time boundary for the segment level.
				t, level, err := segment.ParseTreeKey(treeKey)
				if err == nil {
					if t.After(rp.Levels[level]) {
						rp.Levels[level] = t
					}
				}
			}

			// A key copy must be taken. The slice is reused
			// by iterator but is also used in the batch.
			switch err = batch.Delete(item.KeyCopy(nil)); {
			case err == nil:
			case errors.Is(err, badger.ErrKeyNotFound):
				continue
			default:
				return err
			}

			reclaimed += item.EstimatedSize()
			if removed++; removed%batchSize == 0 {
				if batch, err = s.flushTreeBatch(batch); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Flush remaining items, if any: it's important to make sure
	// all trees were removed before deleting segment nodes - see
	// note on a potential inconsistency above.
	if removed%batchSize != 0 {
		if err = batch.Flush(); err != nil {
			return err
		}
	}

	if len(rp.Levels) > 0 {
		if cached, ok := s.segments.Lookup(k.SegmentKey()); ok {
			if ok, err = cached.(*segment.Segment).DeleteNodesBefore(rp); ok {
				err = s.deleteSegmentAndRelatedData(k)
			}
		}
	}

	return err
}

// flushTreeBatch commits the changes and returns a new batch. The call returns
// the batch unchanged in case of an error so that it can be safely cancelled.
//
// If the storage was requested to close, errClosed will be returned.
func (s *Storage) flushTreeBatch(batch *badger.WriteBatch) (*badger.WriteBatch, error) {
	if err := batch.Flush(); err != nil {
		return batch, err
	}
	select {
	case <-s.stop:
		return batch, errClosed
	default:
		return s.trees.NewWriteBatch(), nil
	}
}

func (s *Storage) iterateOverAllSegments(cb func(*segment.Key) error) error {
	nameKey := "__name__"

	var dimensions []*dimension.Dimension
	s.labels.GetValues(nameKey, func(v string) bool {
		if d, ok := s.lookupAppDimension(v); ok {
			dimensions = append(dimensions, d)
		}
		return true
	})

	for _, r := range dimension.Union(dimensions...) {
		k, err := segment.ParseKey(string(r))
		if err != nil {
			s.logger.WithError(err).WithField("key", string(r)).Error("failed to parse segment key")
			continue
		}
		if err = cb(k); err != nil {
			return err
		}
	}

	return nil
}
