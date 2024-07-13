package querybackend

import (
	"context"

	"github.com/parquet-go/parquet-go"
)

func openProfileTable(ctx context.Context, s *tenantService) (err error) {
	if buf := s.inMemoryBuffer(); buf != nil {
		s.profiles, err = openProfileTableInMemory(ctx, s, buf)
		return err
	}
	s.profiles, err = openProfileTableObject(ctx, s)
	return err
}

func openProfileTableInMemory(ctx context.Context, s *tenantService, buf []byte) (*parquet.File, error) {
	r := newReaderWithOffset(
		s.inMemoryBucket(buf),
		s.sectionOffset(sectionProfiles))
	rat, err := r.ReaderAt(ctx, s.obj.path)
	if err != nil {
		return nil, err
	}
	return parquet.OpenFile(rat, s.sectionSize(sectionProfiles),
		parquet.SkipBloomFilters(true))
}

func openProfileTableObject(ctx context.Context, s *tenantService) (*parquet.File, error) {
	r := newReaderWithOffset(
		s.obj.storage,
		s.sectionOffset(sectionProfiles))
	rat, err := r.ReaderAt(ctx, s.obj.path)
	if err != nil {
		return nil, err
	}
	return parquet.OpenFile(rat, s.sectionSize(sectionProfiles),
		parquet.SkipBloomFilters(true),
		parquet.FileReadMode(parquet.ReadModeAsync),
		parquet.ReadBufferSize(256<<10))
}
