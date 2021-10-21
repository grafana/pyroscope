package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type reclamationRequest struct {
	rp    *segment.RetentionPolicy
	sk    *segment.Key
	sks   string        // Segment key string.
	nodes []segmentNode // Segment nodes to delete.
	size  int           // Capacity to reclaim per segment.
}

type segmentNode struct {
	depth int
	time  int64
}

const batchSize = 1024

// TODO(kolesnikovae): add metrics.

func (s *Storage) Reclaim(ctx context.Context, rp *segment.RetentionPolicy) error {
	if rp.SizeLimit() == 0 {
		return nil
	}

	segmentKeys, err := s.segmentKeys()
	if err != nil || len(segmentKeys) == 0 {
		return err
	}

	var (
		// Occupied disk space. The value should be as relevant as possible,
		// therefore use of Size() is not acceptable (calculated periodically).
		used = dirSize(s.config.StoragePath)
		// Size in bytes to be reclaimed.
		rs = rp.CapacityToReclaim(used, 0.05).Bytes()
		// Size in bytes to be reclaimed per segment.
		rsps = rs / len(segmentKeys)
	)

	logger := s.logger.
		WithField("used", used).
		WithField("requested", rs)

	if rsps == 0 {
		logger.Info("skipping reclaim")
		return nil
	}

	logger.Info("reclaiming disk space")

	// TODO(kolesnikovae): make reclaim fair:
	//   If a segment occupies less than rsps, the leftover
	//   should be then removed with additional request.
	//   Can we run it concurrently with some throttling?
	for k := range segmentKeys {
		r := &reclamationRequest{sks: k, size: rsps}
		if err = s.reclaimSpace(ctx, r); err != nil {
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

	for k, v := range segmentKeys {
		r := &reclamationRequest{rp: rp, sks: k, sk: v}
		if err = s.deleteDataBefore(ctx, r); err != nil {
			return err
		}
	}

	return nil
}

func (s *Storage) reclaimSpace(ctx context.Context, r *reclamationRequest) (err error) {
	var (
		batch     = s.dbTrees.NewWriteBatch()
		removed   int
		reclaimed int
	)
	defer func() {
		err = batch.Flush()
	}()

	// TODO(kolesnikovae):
	//   Don't forget to remove corresponding segment nodes!
	//   Keep track of the most recent node and reuse DeleteNodesBefore
	//   when a batch is successfully committed?  This may(?) result in the
	//   tree still being stored, but not present in the segment; the tree will
	//   be removed from the storage eventually.

	return s.dbTrees.View(func(txn *badger.Txn) error {
		// Lower-level trees come first because of the lexicographical order:
		// from the very first tree to the most recent one.
		it := txn.NewIterator(badger.IteratorOptions{
			// We count all version so that our estimation is more precise
			// but slightly higher than the actual size in practice,
			// meaning that we delete less data (and reclaim less space).
			AllVersions: true,
			// The prefix matches all trees in the segment.
			Prefix: treePrefix.key(r.sks),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if r.size-reclaimed <= 0 {
				return nil
			}

			// A copy must be taken. The slice is reused
			// by iterator but is also used in the batch.
			item := it.Item()
			tk := item.KeyCopy(nil)
			if k, ok := treePrefix.trim(tk); ok {
				s.trees.Discard(string(k))
			}

			switch err = batch.Delete(tk); {
			case err == nil:
			case errors.Is(err, badger.ErrKeyNotFound):
				continue
			default:
				return err
			}

			reclaimed += int(item.EstimatedSize())
			if removed++; removed%batchSize == 0 {
				if batch, err = s.flushTreeBatch(ctx, batch); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (s *Storage) deleteDataBefore(ctx context.Context, r *reclamationRequest) error {
	cached, ok := s.segments.Lookup(r.sks)
	if !ok {
		return nil
	}

	// Instead of removing every tree in an individual transaction for each
	// of them, blocking the segment for a long time, we remember them and
	// drop in batches after the segment is released.
	//
	// To avoid a potential inconsistency when DeleteNodesBefore fails in the
	// process, trees should be removed first. Only after successful commit,
	// segment nodes can be safely removed, to guaranty eventual idempotency.

	// There is a better way of removing these trees: we could calculate
	// non-overlapping prefixes for depth levels, e.g:
	//     0:163474....
	//     1:1634745...
	//     2:16347456..
	//
	// And drop the data (both in cache and on disk) using these prefixes.
	// Remaining trees (with overlapping prefixes) would be removed in batches.
	// That would be especially efficient in cases when data removed for a very
	// long period. For example, when retention-period is enabled for the first
	// time on a server with historical data.

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

	var removed int
	batch := s.dbTrees.NewWriteBatch()
	defer batch.Cancel()

	for _, n := range r.nodes {
		treeKey := segment.TreeKey(r.sks, n.depth, n.time)
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
			if batch, err = s.flushTreeBatch(ctx, batch); err != nil {
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

	_, err = seg.DeleteNodesBefore(r.rp)
	return err
}

// flushTreeBatch commits the changes and returns a new batch.
func (s *Storage) flushTreeBatch(ctx context.Context, batch *badger.WriteBatch) (*badger.WriteBatch, error) {
	if err := batch.Flush(); err != nil {
		return batch, err
	}
	return s.dbTrees.NewWriteBatch(), ctx.Err()
}

// segmentKeys returns a map of valid segment keys found in the storage.
// It returns both parsed, and string representation.
func (s *Storage) segmentKeys() (map[string]*segment.Key, error) {
	segmentKeys := make(map[string]*segment.Key)
	return segmentKeys, s.dbSegments.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			Prefix: segmentPrefix.bytes(),
		})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if k, ok := segmentPrefix.trim(it.Item().Key()); ok {
				// k must not be reused outside of the transaction.
				key, err := segment.ParseKey(string(k))
				if err != nil {
					continue
				}
				segmentKeys[key.Normalized()] = key
			}
		}
		return nil
	})
}

// TODO(kolesnikovae): filepath.Walk is notoriously slow.
//  Consider use of https://github.com/karrick/godirwalk.
//  Although, every badger.DB calculates its size (reported
//  via Size) in the same way every minute.
func dirSize(path string) bytesize.ByteSize {
	var result int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			result += info.Size()
		}
		return nil
	})
	return bytesize.ByteSize(result)
}
