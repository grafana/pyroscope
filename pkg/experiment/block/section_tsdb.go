package block

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
)

func openTSDB(ctx context.Context, s *Dataset) (err error) {
	offset := s.sectionOffset(SectionTSDB)
	size := s.sectionSize(SectionTSDB)
	s.tsdb = new(tsdbBuffer)
	defer func() {
		if err != nil {
			_ = s.tsdb.Close()
		}
	}()
	if buf := s.inMemoryBuffer(); buf != nil {
		offset -= int64(s.offset())
		s.tsdb.index, err = index.NewReader(index.RealByteSlice(buf[offset : offset+size]))
	} else {
		s.tsdb.buf = bufferpool.GetBuffer(int(size))
		if err = objstore.ReadRange(ctx, s.tsdb.buf, s.obj.path, s.obj.storage, offset, size); err == nil {
			s.tsdb.index, err = index.NewReader(index.RealByteSlice(s.tsdb.buf.B))
		}
	}
	if err != nil {
		return fmt.Errorf("opening tsdb: %w", err)
	}
	return nil
}

type tsdbBuffer struct {
	index *index.Reader
	buf   *bufferpool.Buffer
}

func (b *tsdbBuffer) Close() (err error) {
	if b.index != nil {
		err = b.index.Close()
		b.index = nil
	}
	if b.buf != nil {
		bufferpool.Put(b.buf)
		b.buf = nil
	}
	return err
}
