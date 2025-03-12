package pprof

import (
	"bufio"
	"compress/gzip"
	"fmt"
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"io"

	"github.com/valyala/bytebufferpool"
)

var bufPool = bytebufferpool.Pool{}

func Decode(r io.Reader, p *profilev1.Profile) error {
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
	buf := bufPool.Get()
	defer bufPool.Put(buf)
	if _, err = io.Copy(buf, r); err != nil {
		return err
	}
	return p.UnmarshalVT(buf.Bytes())
}
