package block

import (
	"bytes"
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func openTSDB(ctx context.Context, s *TenantService) (err error) {
	offset := s.sectionOffset(SectionTSDB)
	size := s.sectionSize(SectionTSDB)
	if buf := s.inMemoryBuffer(); buf != nil {
		offset -= int64(s.offset())
		s.TSDB, err = index.NewReader(index.RealByteSlice(buf[offset : offset+size]))
	} else {
		// TODO(kolesnikovae): This buffer should be reused.
		//  Caveat: objects returned by tsdb may reference the buffer
		//  and be still in use after the object is closed.
		var dst bytes.Buffer
		if err = objstore.FetchRange(ctx, &dst, s.obj.path, s.obj.storage, offset, size); err == nil {
			s.TSDB, err = index.NewReader(index.RealByteSlice(dst.Bytes()))
		}
	}
	if err != nil {
		return fmt.Errorf("opening tsdb: %w", err)
	}
	return nil
}
