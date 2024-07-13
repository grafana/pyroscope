package querybackend

import (
	"bytes"
	"context"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func openTSDB(ctx context.Context, s *tenantService) (err error) {
	if buf := s.inMemoryBuffer(); buf != nil {
		s.tsdb, err = openTSDBInMemory(ctx, s, buf)
		return err
	}
	s.tsdb, err = openTSDBObject(ctx, s)
	return err
}

func openTSDBInMemory(_ context.Context, s *tenantService, buf []byte) (*index.Reader, error) {
	off := s.sectionOffset(sectionTSDB)
	size := s.sectionSize(sectionTSDB)
	return index.NewReader(index.RealByteSlice(buf[off : off+size]))
}

func openTSDBObject(ctx context.Context, s *tenantService) (*index.Reader, error) {
	off := s.sectionOffset(sectionTSDB)
	size := s.sectionSize(sectionTSDB)
	buf := new(bytes.Buffer)
	if err := objstore.FetchRange(ctx, buf, s.obj.path, s.obj.storage, off, size); err != nil {
		return nil, err
	}
	return index.NewReader(index.RealByteSlice(buf.Bytes()))
}
