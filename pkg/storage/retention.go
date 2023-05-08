package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

const defaultBatchSize = 1 << 10 // 1K items

func (s *Storage) enforceRetentionPolicy(ctx context.Context, rp *segment.RetentionPolicy) {
	observer := prometheus.ObserverFunc(s.metrics.retentionTaskDuration.Observe)
	timer := prometheus.NewTimer(observer)
	defer timer.ObserveDuration()

	s.logger.Debug("enforcing retention policy")
	err := s.iterateOverAllSegments(func(k *segment.Key) error {
		return s.deleteSegmentData(ctx, k, rp)
	})

	switch {
	case err == nil:
	case errors.Is(ctx.Err(), context.Canceled):
		s.logger.Warn("enforcing retention policy canceled")
	default:
		s.logger.WithError(err).Error("failed to enforce retention policy")
	}
}

func (s *Storage) deleteSegmentData(ctx context.Context, k *segment.Key, rp *segment.RetentionPolicy) error {
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
	defer func() {
		if removed > 0 {
			sk := k.SegmentKey()
			query := fmt.Sprintf("ALTER TABLE segment DROP PARTITION IF EXISTS %s", sk)
			_, err := s.trees.Exec(query)
			if err != nil {
				// log.Errorf("error dropping segment partition %s: %v", sk, err)
			}
		}
	}()

	for _, n := range nodes {
		treeKey := segment.TreeKey(sk, n.depth, n.time)
		s.trees.Discard(treeKey)

		if removed++; removed%defaultBatchSize == 0 {
			query := fmt.Sprintf("ALTER TABLE segment DROP PARTITION IF EXISTS %s", sk)
			_, err = s.trees.Exec(query)
			if err != nil {
				// log.Errorf("error dropping segment partition %s: %v", sk, err)
			}
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
	var myInt64 int64 = 10000
	batchSize := myInt64
	var (
		removed   int64
		reclaimed int64
		err       error
	)

	// Keep track of the most recent removed tree time per every segment level.
	rp := &segment.RetentionPolicy{Levels: make(map[int]time.Time)}

	// Lower-level trees come first because of the lexicographical order:
	// from the very first tree to the most recent one, from the lowest
	// level (with highest resolution) to the highest.
	rows, err := s.main.Query("SELECT * FROM trees WHERE segment_key = ? ORDER BY tree_key", k.SegmentKey())
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if size-reclaimed <= 0 {
			break
		}

		var (
			treeKey     string
			treeEstSize int64
		)
		err = rows.Scan(&treeKey, &treeEstSize)
		if err != nil {
			return err
		}
		s.trees.Discard(treeKey)
		// Update the time boundary for the segment level.
		t, level, err := segment.ParseTreeKey(treeKey)
		if err == nil {
			if t.After(rp.Levels[level]) {
				rp.Levels[level] = t
			}
		}

		_, err = s.main.Exec("DELETE FROM trees WHERE tree_key = ?", treeKey)
		if err != nil {
			return err
		}

		reclaimed += treeEstSize
		if removed++; removed%batchSize == 0 {
			// Commit the current batch.
			if _, err = s.main.Exec("COMMIT"); err != nil {
				return err
			}
		}
	}

	if removed%batchSize != 0 {
		// Commit any remaining batch.
		if _, err = s.main.Exec("COMMIT"); err != nil {
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
// func (s *Storage) flushTreeBatch(batch *sql.Tx) (*sql.Tx, error) {
// 	if err := batch.Commit(); err != nil {
// 		return batch, err
// 	}
// 	select {
// 	case <-s.stop:
// 		return batch, errClosed
// 	default:
// 		return s.main, nil
// 	}
// }

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
