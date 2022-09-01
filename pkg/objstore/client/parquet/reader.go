package parquet

import (
	"encoding/binary"

	"github.com/grafana/fire/pkg/objstore"
)

type parquetReaderAt struct {
	objstore.ReaderAt
	footerSize uint32
}

func NewReaderAt(r objstore.ReaderAt) objstore.ReaderAt {
	return &parquetReaderAt{
		ReaderAt: r,
	}
}

// called by parquet-go in OpenFile() to set offset and length of footer section
func (r *parquetReaderAt) SetFooterSection(offset, length int64) {
	// todo cache footer section
}

// called by parquet-go in OpenFile() to set offset and length of column indexes
func (r *parquetReaderAt) SetColumnIndexSection(offset, length int64) {
	// todo cache column index section
}

// called by parquet-go in OpenFile() to set offset and length of offset index section
func (r *parquetReaderAt) SetOffsetIndexSection(offset, length int64) {
	// todo cache offset index section
}

func (r *parquetReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 4 && off == 0 {
		// Magic header
		return copy(p, []byte("PAR1")), nil
	}

	if len(p) == 8 && off == r.Size()-8 && r.footerSize > 0 /* not present in previous block metas */ {
		// Magic footer
		binary.LittleEndian.PutUint32(p, r.footerSize)
		copy(p[4:8], []byte("PAR1"))
		return 8, nil
	}

	// todo handle cache
	return r.ReaderAt.ReadAt(p, off)
}
