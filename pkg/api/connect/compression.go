package connectapi

import (
	"io"

	"connectrpc.com/connect"
	"github.com/klauspost/compress/gzip"

	"github.com/grafana/pyroscope/v2/pkg/util/gzipwriter"
)

const (
	compressionGzip = "gzip"
)

var (
	gzipPoolHandler = connect.WithCompression(
		compressionGzip,
		func() connect.Decompressor { return &gzip.Reader{} },
		func() connect.Compressor { return gzipwriter.NewAdaptiveWriter(io.Discard) },
	)
	gzipPoolClient = connect.WithAcceptCompression(
		compressionGzip,
		func() connect.Decompressor { return &gzip.Reader{} },
		func() connect.Compressor { return gzipwriter.NewAdaptiveWriter(io.Discard) },
	)
)

func WithGzipHandler() connect.HandlerOption {
	return gzipPoolHandler
}

func WithGzipClient() connect.ClientOption {
	return gzipPoolClient
}
