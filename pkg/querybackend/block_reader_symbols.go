package querybackend

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func openSymbols(ctx context.Context, s *tenantService) (err error) {
	offset := s.sectionOffset(sectionSymbols)
	size := s.sectionSize(sectionSymbols)
	if buf := s.inMemoryBuffer(); buf != nil {
		reader := objstore.NewBucketReaderWithOffset(s.inMemoryBucket(buf), offset)
		s.symbols, err = symdb.OpenObject(ctx, reader, s.obj.path, size)
	} else {
		reader := objstore.NewBucketReaderWithOffset(s.obj.storage, offset)
		s.symbols, err = symdb.OpenObject(ctx, reader, s.obj.path, size,
			symdb.WithPrefetchSize(32<<10))
	}
	if err != nil {
		return fmt.Errorf("opening symbols: %w", err)
	}
	return nil
}
