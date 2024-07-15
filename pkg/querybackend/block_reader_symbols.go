package querybackend

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func openSymbols(ctx context.Context, s *tenantService) (err error) {
	if buf := s.inMemoryBuffer(); buf != nil {
		s.symbols, err = symdb.OpenObject(ctx,
			s.inMemoryBucket(buf),
			s.obj.path,
			s.sectionOffset(sectionSymbols),
			s.sectionSize(sectionSymbols))
	} else {
		s.symbols, err = symdb.OpenObject(ctx,
			s.obj.storage,
			s.obj.path,
			s.sectionOffset(sectionSymbols),
			s.sectionSize(sectionSymbols))
	}
	if err != nil {
		return fmt.Errorf("opening symbols: %w", err)
	}
	return nil
}
