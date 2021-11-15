package storage

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

type DeleteInput struct {
	// Key must match exactly one segment.
	Key *segment.Key
}

func (s *Storage) Delete(di *DeleteInput) error {
	return s.deleteSegmentAndRelatedData(di.Key)
}

func (s *Storage) deleteSegmentAndRelatedData(k *segment.Key) error {
	sk := k.SegmentKey()
	if _, ok := s.segments.Lookup(sk); !ok {
		return nil
	}
	// Drop trees from disk.
	if err := s.trees.DropPrefix(treePrefix.key(sk)); err != nil {
		return err
	}
	// Discarding cached items is necessary because otherwise
	// those would be written back to disk on eviction.
	s.trees.DiscardPrefix(sk)
	for key, value := range k.Labels() {
		d, ok := s.lookupDimensionKV(key, value)
		if !ok {
			continue
		}
		d.Delete(dimension.Key(sk))
		if len(d.Keys) > 0 {
			continue
		}
		// There are no more references.
		if err := s.labels.Delete(key, value); err != nil {
			return err
		}
		if key == "__name__" {
			if err := s.dicts.Delete(k.DictKey()); err != nil {
				return err
			}
		}
	}
	return s.segments.Delete(sk)
}
