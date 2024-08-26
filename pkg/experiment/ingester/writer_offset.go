package ingester

import (
	"io"
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
