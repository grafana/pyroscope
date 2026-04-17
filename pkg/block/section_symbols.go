package block

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func openSymbols(ctx context.Context, s *Dataset) (err error) {
	offset := s.sectionOffset(SectionSymbols)
	size := s.sectionSize(SectionSymbols)
	if buf := s.inMemoryBuffer(); buf != nil {
		offset -= int64(s.offset())
		s.symbols, err = symdb.OpenObject(ctx, s.inMemoryBucket(buf), s.obj.path, offset, size)
	} else {
		opts := []symdb.Option{symdb.WithPrefetchSize(symbolsPrefetchSize)}
		if s.obj.hedgeAfter > 0 {
			opts = append(opts, symdb.WithHedgeAfter(s.obj.hedgeAfter))
		}
		s.symbols, err = symdb.OpenObject(ctx, s.obj.storage, s.obj.path, offset, size, opts...)
	}
	if err != nil {
		return fmt.Errorf("opening symbols: %w", err)
	}
	return nil
}
