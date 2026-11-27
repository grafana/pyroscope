package block

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

func openSymbols(ctx context.Context, s *Dataset) (err error) {
	offset := s.sectionOffset(SectionSymbols)
	size := s.sectionSize(SectionSymbols)
	if buf, relOff := s.inMemorySection(SectionSymbols); buf != nil {
		s.symbols, err = symdb.OpenObject(ctx, s.inMemoryBucket(buf), s.obj.path, relOff, size)
	} else {
		s.symbols, err = symdb.OpenObject(ctx, s.obj.storage, s.obj.path, offset, size,
			symdb.WithPrefetchSize(symbolsPrefetchSize))
	}
	if err != nil {
		return fmt.Errorf("opening symbols: %w", err)
	}
	return nil
}
