package storage

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

func (s *Storage) Reclaim(ctx context.Context, rp *segment.RetentionPolicy) error {
	segmentKeys, err := s.segmentKeys()
	if err != nil || len(segmentKeys) == 0 {
		return err
	}

	var (
		// Size in bytes to be reclaimed.
		rs = s.capacityToReclaim().Bytes()
		// Size in bytes to be reclaimed per segment, if applicable.
		rsps = rs / len(segmentKeys)
		// Counters for deleted trees.
		stats reclaimStats
	)

	for k, v := range segmentKeys {
		if rp.LowerTimeBoundary() != zeroTime {
			if err = s.purgeObsolete(ctx, &stats, rp, k, v); err != nil {
				// TODO: log and continue.
				return err
			}
		}

		// Remaining part is only needed when a size limit is applied.
		if rs <= 0 {
			continue
		}

		// TODO
		// If time-based retention reclaimed more space than requested
		// for a given segment, adjust initial assumptions so that less
		// data will be removed for remaining segments.
		// if rsps-reclaimed <= 0 {
		// 	continue
		// }

		// There are more trees to be removed to reclaim space.
		if err = s.reclaimSpace(ctx, &stats, k, rsps); err != nil {
			// TODO: log and continue.
			return err
		}
	}

	return nil
}

type reclaimStats struct {
	removedTrees            int
	estimatedReclaimedSpace int
}

const batchSize = 1024

func (s *Storage) reclaimSpace(ctx context.Context, stats *reclaimStats, segmentKey string, size int) (err error) {
	var (
		batch     = s.dbTrees.NewWriteBatch()
		removed   int
		reclaimed int
	)
	defer func() {
		err = batch.Flush()
		stats.removedTrees += removed
		stats.estimatedReclaimedSpace += reclaimed
	}()

	return s.dbTrees.View(func(txn *badger.Txn) error {
		// Lower-level trees go first because of the lexicographical order.
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			// The prefix matches all trees of the segment.
			Prefix: []byte("t:" + segmentKey),
			// We count all version so that our estimation is more precise
			// but slightly higher than the actual size in practice,
			// meaning that we delete less data (and reclaim less space).
			AllVersions: true,
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if size-reclaimed <= 0 {
				return nil
			}

			item := it.Item()
			tk := item.KeyCopy(nil)
			// A copy must be taken: the slice is reused by iterator.
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

			// A sanity check: a tree key can not be shorter 18 bytes:
			// t:<segment_key>{}:0:1234567890
			if len(tk) > 18 {
				s.trees.RemoveFromCache(string(tk[2:]))
				// TODO: remove corresponding segment nodes
			}

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

func (s *Storage) purgeObsolete(
	ctx context.Context,
	stats *reclaimStats,
	rp *segment.RetentionPolicy,
	segmentKey string,
	k *segment.Key) (err error) {

	var (
		batch     = s.dbTrees.NewWriteBatch()
		removed   int
		reclaimed int
	)
	defer func() {
		err = batch.Flush()
		stats.removedTrees += removed
		stats.estimatedReclaimedSpace += reclaimed
	}()

	return s.dbTrees.View(func(txn *badger.Txn) error {
		cached, ok := s.segments.Lookup(segmentKey)
		if !ok {
			return nil
		}
		seg := cached.(*segment.Segment)
		deletedRoot, delErr := seg.DeleteDataBefore(rp, func(depth int, t time.Time) error {
			tk := segmentKey + ":" + strconv.Itoa(depth) + ":" + strconv.Itoa(int(t.Unix()))
			dbk := []byte("t:" + tk)

			// Fetch item to know it's size.
			// TODO: estimate based on cache statistics?
			var item *badger.Item
			switch item, err = txn.Get(dbk); {
			case err == nil:
			case errors.Is(err, badger.ErrKeyNotFound):
				return nil
			default:
				return err
			}
			switch err = batch.Delete(dbk); {
			case err == nil:
			case errors.Is(err, badger.ErrKeyNotFound):
				return nil
			default:
				return err
			}

			reclaimed += int(item.EstimatedSize())
			if removed++; removed%batchSize != 0 {
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

			// A sanity check: a tree key can not be shorter 18 bytes:
			// t:<segment_key>{}:0:1234567890
			if len(tk) > 18 {
				s.trees.RemoveFromCache(string(tk[2:]))
			}

			s.trees.RemoveFromCache(tk)

			return nil
		})
		if delErr == nil && deletedRoot == true {
			return s.deleteSegmentAndRelatedData(k)
		}
		return delErr
	})
}

// capacityToReclaim reports disk space capacity in bytes needs to be reclaimed.
// The call returns non-zero value when 95% of capacity is already in use or if
// less than 1GB of space is available.
func (s *Storage) capacityToReclaim() bytesize.ByteSize {
	if s.config.RetentionSize == 0 {
		return 0
	}
	x := bytesize.ByteSize(float64(s.config.RetentionSize) * 0.05)
	if x > bytesize.GB {
		x = bytesize.GB
	}
	if v := dirSize(s.config.StoragePath) + x - s.config.RetentionSize; v > 0 {
		return v
	}
	return 0
}

func (s *Storage) segmentKeys() (map[string]*segment.Key, error) {
	segmentKeys := make(map[string]*segment.Key)
	return segmentKeys, s.dbSegments.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			Prefix:         []byte("s:"),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			// TODO: check key
			k := item.KeyCopy(nil)
			if len(k) > 2 {
				sk := string(k[2:])
				key, err := segment.ParseKey(sk)
				if err != nil {
					continue
				}
				segmentKeys[sk] = key
			}
		}
		return nil
	})
}
