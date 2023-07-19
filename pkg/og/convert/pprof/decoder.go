package pprof

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/streaming"
	"io"

	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

func Decode(r io.Reader, p *tree.Profile) error {
	br := bufio.NewReader(r)
	header, err := br.Peek(2)
	if err != nil {
		return fmt.Errorf("failed to read pprof profile header: %w", err)
	}
	if header[0] == 0x1f && header[1] == 0x8b {
		gzipr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("failed to create pprof profile zip reader: %w", err)
		}
		r = gzipr
		defer gzipr.Close()
	} else {
		r = br
	}
	buf := streaming.PPROFBufPool.Get()
	defer streaming.PPROFBufPool.Put(buf)
	if _, err = io.Copy(buf, r); err != nil {
		return err
	}
	return p.UnmarshalVT(buf.Bytes())
}

func DecodePool(r io.Reader, fn func(*tree.Profile) error) error {
	p := tree.ProfileFromVTPool()
	defer p.ReturnToVTPool()
	p.Reset()
	if err := Decode(r, p); err != nil {
		return err
	}
	return fn(p)
}
