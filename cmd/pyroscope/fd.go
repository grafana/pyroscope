package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/gzip"
)

type gzipBuffer struct {
	gzr gzip.Reader
	gzw *gzip.Writer

	out          bytes.Buffer
	uncompressed bytes.Buffer
	in           *bytes.Reader
}

var gzipBufferPool = sync.Pool{
	New: func() interface{} {
		return &gzipBuffer{
			gzw: gzip.NewWriter(nil),
			in:  bytes.NewReader(nil),
		}
	},
}

func getGzipBuffer() *gzipBuffer {
	buf := gzipBufferPool.Get().(*gzipBuffer)
	buf.reset()
	return buf
}

func putGzipBuffer(buf *gzipBuffer) {
	gzipBufferPool.Put(buf)
}

func (d *gzipBuffer) reset() io.Writer {
	d.out.Reset()
	d.gzw.Reset(&d.out)
	return d.gzw
}

func (d *gzipBuffer) uncompress(in []byte) ([]byte, error) {
	if !isGzipData(in) {
		return in, nil
	}
	d.in.Reset(in)
	if err := d.gzr.Reset(d.in); err != nil {
		return nil, err
	}
	d.uncompressed.Reset()
	d.uncompressed.Grow(uncompressedSize(in))
	_, err := d.uncompressed.ReadFrom(&d.gzr)
	if err != nil {
		return nil, fmt.Errorf("decompressing profile: %v", err)
	}
	return d.uncompressed.Bytes(), nil
}

func isGzipData(data []byte) bool {
	return bytes.HasPrefix(data, []byte{0x1f, 0x8b})
}

func uncompressedSize(in []byte) int {
	last := len(in)
	if last < 4 {
		return -1
	}
	return int(binary.LittleEndian.Uint32(in[last-4 : last]))
}

type DeltaProfiler interface {
	Delta(p []byte, out io.Writer) error
}

// computeDelta computes the delta between the given profile and the last
// data is uncompressed if it is gzip compressed.
// The returned data is always gzip compressed.
func computeDelta(delta DeltaProfiler, data []byte) (b []byte, err error) {
	gzipBuf := getGzipBuffer()
	defer putGzipBuffer(gzipBuf)

	data, err = gzipBuf.uncompress(data)
	if err != nil {
		return nil, err
	}

	if err = delta.Delta(data, gzipBuf.gzw); err != nil {
		return nil, fmt.Errorf("computing delta: %v", err)
	}
	if err := gzipBuf.gzw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %v", err)
	}
	// The returned slice will be retained in case the profile upload fails,
	// so we need to return a copy of the buffer's bytes to avoid a data
	// race.
	b = make([]byte, len(gzipBuf.out.Bytes()))
	copy(b, gzipBuf.out.Bytes())
	return b, nil
}
