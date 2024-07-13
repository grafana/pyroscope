package querybackend

import (
	"context"

	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func openSymbols(ctx context.Context, s *tenantService) (err error) {
	if buf := s.inMemoryBuffer(); buf != nil {
		s.symbols, err = openSymbolsInMemory(ctx, s, buf)
		return err
	}
	s.symbols, err = openSymbolsObject(ctx, s)
	return err
}

func openSymbolsInMemory(ctx context.Context, s *tenantService, buf []byte) (*symdb.Reader, error) {
	r := newReaderWithOffset(
		s.inMemoryBucket(buf),
		s.sectionOffset(sectionSymbols))
	return symdb.OpenReaderV3(ctx, r)
}

func openSymbolsObject(ctx context.Context, s *tenantService) (*symdb.Reader, error) {
	r := newReaderWithOffset(
		s.obj.storage,
		s.sectionOffset(sectionSymbols))
	return symdb.OpenReaderV3(ctx, r)
}
