package ingester

import (
	"io"
	"os"

	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type writerOffset struct {
	io.Writer
	offset int64
	//err    error
}

func withWriterOffset(w io.Writer) *writerOffset {
	return &writerOffset{Writer: w}
}

//func (w *writerOffset) write(p []byte) {
//	if w.err == nil {
//		n, err := w.Writer.Write(p)
//		w.offset += int64(n)
//		w.err = err
//	}
//}

func (w *writerOffset) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	w.offset += int64(n)
	return n, err
}

func concatFile(w *writerOffset, h *phlaredb.Head, f *block.File) (uint64, error) {
	o := w.offset
	fp := h.LocalPathFor(f.RelPath)
	file, err := os.Open(fp)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	_, err = io.Copy(w, file)
	if err != nil {
		return 0, err
	}
	return uint64(o), nil
}

func concatFiles(w *writerOffset, h *phlaredb.Head, f ...*block.File) ([]uint64, error) {
	res := make([]uint64, len(f))
	for i, file := range f {
		o, err := concatFile(w, h, file)
		if err != nil {
			return nil, err
		}
		res[i] = o
	}
	return res, nil
}
