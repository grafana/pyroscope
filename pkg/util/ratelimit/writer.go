package ratelimit

import (
	"io"
)

type Writer struct {
	w io.Writer
	l *Limiter
}

func NewWriter(w io.Writer, l *Limiter) *Writer {
	return &Writer{w: w, l: l}
}

func (rw *Writer) Write(p []byte) (int, error) {
	var total int
	for len(p) > 0 {
		n := len(p)
		if n > int(rw.l.rate) {
			n = int(rw.l.rate)
		}
		rw.l.Wait(n)
		written, err := rw.w.Write(p[:n])
		total += written
		p = p[written:]
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
