package querybackend

import (
	"bytes"
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func openTSDB(ctx context.Context, s *tenantService) (err error) {
	off := s.sectionOffset(sectionTSDB)
	size := s.sectionSize(sectionTSDB)
	if buf := s.inMemoryBuffer(); buf != nil {
		s.tsdb, err = index.NewReader(index.RealByteSlice(buf[off : off+size]))
	} else {
		// TODO(kolesnikovae): This buffer should be reused.
		//  Caveat: objects returned by tsdb may reference the buffer
		//  and be still in use after the object is closed.
		var dst bytes.Buffer
		if err = objstore.FetchRange(ctx, &dst, s.obj.path, s.obj.storage, off, size); err == nil {
			s.tsdb, err = index.NewReader(index.RealByteSlice(dst.Bytes()))
		}
	}
	if err != nil {
		return fmt.Errorf("opening tsdb: %w", err)
	}
	return nil
}
