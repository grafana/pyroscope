package block

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func openSymbols(ctx context.Context, s *TenantService) (err error) {
	offset := s.sectionOffset(SectionSymbols)
	size := s.sectionSize(SectionSymbols)
	if buf := s.inMemoryBuffer(); buf != nil {
		offset -= int64(s.offset())
		reader := objstore.NewBucketReaderWithOffset(s.inMemoryBucket(buf), offset)
		s.Symbols, err = symdb.OpenObject(ctx, reader, s.obj.path, size)
	} else {
		reader := objstore.NewBucketReaderWithOffset(s.obj.storage, offset)
		s.Symbols, err = symdb.OpenObject(ctx, reader, s.obj.path, size,
			symdb.WithPrefetchSize(32<<10))
	}
	if err != nil {
		return fmt.Errorf("opening symbols: %w", err)
	}
	return nil
}
