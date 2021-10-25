package storage

import (
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

const batchSize = 1024

// TODO(kolesnikovae): add metrics.

func (s *Storage) CollectGarbage() {
	s.logger.Info("starting garbage collection")
	rp := s.retentionPolicy()
	var segmentKeys []*segment.Key

	if !rp.LowerTimeBoundary().IsZero() {
		s.logger.Info("enforcing time-based retention policy")
		var err error
		if segmentKeys, err = s.segmentKeys(); err != nil {
			s.logger.WithError(err).Error("failed to fetch segment keys")
			return
		}
		// TODO(kolesnikovae): Should it run concurrently with some throttling?
		for _, k := range segmentKeys {
			switch err = s.deleteSegmentDataBefore(k, rp); err {
			default:
				s.logger.WithError(err).WithField("segment", k).
					Warn("failed to enforce time-based retention policy")
			case nil:
			case errClosed:
				s.logger.WithField("segment", k).
					Warn("enforcing time-based retention policy canceled")
				return
			}
		}
	}

	if !s.main.runGC(0.000001) {
		s.logger.Info("garbage collection did not result in reclaimed space")
		return
	}

	// Occupied disk space. The value should be as accurate
	// as possible and needs to be updated when GC succeeds.
	s.size = s.calculateDBSize()
	// No need to proceed if there is no limit on the db size.
	if rp.SizeLimit() == 0 {
		return
	}

	// The volume to reclaim is determined by approximation on the key-value
	// pairs size, which is very close to the actual occupied disk space only
	// when garbage collector has discarded unclaimed space in value log files.
	//
	// At this point size estimations should be quite precise and we can remove
	// items from the database safely. Effectively, only trees are removed to
	// reclaim space in accordance to the size-based retention policy.
	var (
		// Size in bytes to be reclaimed.
		rs = rp.CapacityToReclaim(s.size, s.reclaimSizeRatio)
		// Size in bytes to be reclaimed per segment.
		rsps = rs.Bytes() / len(segmentKeys)
	)

	logger := s.logger.WithFields(logrus.Fields{
		"used":      s.size,
		"requested": rs,
	})

	if rsps == 0 {
		logger.Info("skipping reclaim")
		return
	}

	// TODO(kolesnikovae): make reclaim more fair:
	//   If a segment occupies less than rsps, the leftover
	//   should be then removed with an additional request.
	//   Another point is that if the procedure has been
	//   interrupted, data will be removed disproportionally.

	s.logger.Info("enforcing size-based retention policy")
	for _, k := range segmentKeys {
		switch err := s.reclaimSegmentSpace(k, rsps); err {
		default:
			s.logger.WithError(err).WithField("segment", k).
				Warn("failed to enforce size-based retention policy")
		case nil:
		case errClosed:
			s.logger.WithField("segment", k).
				Warn("enforcing size-based retention policy canceled")
			return
		}
	}
}

func (s *Storage) deleteSegmentDataBefore(k *segment.Key, rp *segment.RetentionPolicy) error {
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

	var removed int
	batch := s.trees.NewWriteBatch()
	defer batch.Cancel()

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

func (s *Storage) reclaimSegmentSpace(k *segment.Key, size int) error {
	batch := s.trees.NewWriteBatch()
	defer batch.Cancel() // TODO(kolesnikovae): make sure argument is captured.
	var (
		removed   int
		reclaimed int
		err       error
	)

	// Keep track of the most recent removed tree time per every segment level.
	rp := &segment.RetentionPolicy{Levels: make(map[int]time.Time)}
	err = s.trees.View(func(txn *badger.Txn) error {
		// Lower-level trees come first because of the lexicographical order:
		// from the very first tree to the most recent one.
		it := txn.NewIterator(badger.IteratorOptions{
			// We count all version so that our estimation is more precise
			// but slightly higher than the actual size in practice,
			// meaning that we delete less data (and reclaim less space).
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
				// Update time boundary for the segment level.
				t, level, err := segment.ParseTreeKey(treeKey)
				if err != nil {
					return fmt.Errorf("unable to parse tree key %q: %w", treeKey, err)
				}
				if t.After(rp.Levels[level]) {
					rp.Levels[level] = t
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

			reclaimed += int(item.EstimatedSize())
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
// the batch unchanged in case of an error.
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

// segmentKeys returns a map of valid segment keys found in the storage.
// It returns both parsed, and string representation.
func (s *Storage) segmentKeys() ([]*segment.Key, error) {
	keys := make([]*segment.Key, 0)
	return keys, s.segments.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			Prefix: segmentPrefix.bytes(),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if k, ok := segmentPrefix.trim(it.Item().Key()); ok {
				// k must not be reused outside of the transaction.
				if key, err := segment.ParseKey(string(k)); err == nil {
					keys = append(keys, key)
				}
			}
		}
		return nil
	})
}

func (s *Storage) retentionPolicy() *segment.RetentionPolicy {
	t := segment.NewRetentionPolicy().
		SetAbsoluteMaxAge(s.config.Retention).
		SetSizeLimit(s.config.RetentionSize)
	for level, threshold := range s.config.RetentionLevels {
		t.SetLevelMaxAge(level, threshold)
	}
	return t
}
