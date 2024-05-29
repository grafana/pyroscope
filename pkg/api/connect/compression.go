package connectapi

import (
	"io"

	"connectrpc.com/connect"
	"github.com/klauspost/compress/gzip"
	"github.com/pierrec/lz4/v4"
)

const (
	compressionGzip = "gzip"
	compressionLZ4  = "lz4"
)

var (
	gzipPoolHandler = connect.WithCompression(
		compressionGzip,
		func() connect.Decompressor { return &gzip.Reader{} },
		func() connect.Compressor { return gzip.NewWriter(io.Discard) },
	)
	gzipPoolClient = connect.WithAcceptCompression(
		compressionGzip,
		func() connect.Decompressor { return &gzip.Reader{} },
		func() connect.Compressor { return gzip.NewWriter(io.Discard) },
	)
)

func WithGzipHandler() connect.HandlerOption {
	return gzipPoolHandler
}

func WithGzipClient() connect.ClientOption {
	return gzipPoolClient
}

func newLZ4Reader(r io.Reader) connect.Decompressor {
	xr := lz4.NewReader(r)
	return &lz4Reader{xr}
}

type lz4Reader struct {
	*lz4.Reader
}

func (r *lz4Reader) Close() error {
	r.Reader.Reset(nil)
	return nil
}

func (r *lz4Reader) Reset(src io.Reader) error {
	r.Reader.Reset(src)
	return nil
}

var (
	lz4PoolHandler = connect.WithCompression(
		compressionLZ4,
		func() connect.Decompressor { return newLZ4Reader(nil) },
		func() connect.Compressor { return lz4.NewWriter(io.Discard) },
	)
	lz4PoolClient = connect.WithAcceptCompression(
		compressionLZ4,
		func() connect.Decompressor { return newLZ4Reader(nil) },
		func() connect.Compressor { return lz4.NewWriter(io.Discard) },
	)
)

func WithLZ4Handler() connect.HandlerOption {
	return lz4PoolHandler
}

func WithLZ4Client() connect.ClientOption {
	return lz4PoolClient
}

func WithoutCompressionHandler() connect.HandlerOption {
	return connect.WithCompression(
		compressionGzip,
		nil,
		nil,
	)
}
func WithoutCompressionClient() connect.ClientOption {
	return connect.WithAcceptCompression(
		compressionGzip,
		nil,
		nil,
	)
}
