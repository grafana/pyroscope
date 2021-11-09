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
	// Discarding cached items is necessary because otherwise those would
	// be written back to disk on eviction.
	s.trees.DiscardPrefix(sk)
	// Only remove dictionary if there are no more segments referencing it.
	if apps, ok := s.lookupAppDimension(k.AppName()); ok && len(apps.Keys) == 1 {
		if err := s.dicts.Delete(k.DictKey()); err != nil {
			return err
		}
	}
	for key, value := range k.Labels() {
		if d, ok := s.lookupDimensionKV(key, value); ok {
			d.Delete(dimension.Key(sk))
		}
	}
	return s.segments.Delete(sk)
}
