package util

import (
	"fmt"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/gzip"
	"github.com/parquet-go/parquet-go/compress/lz4"
	"github.com/parquet-go/parquet-go/compress/snappy"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

// ParseCompressionOpt parse parquet compression option
func ParseCompressionOpt(algo string, level int) (parquet.WriterOption, error) {
	switch algo {
	case "gzip":
		if level == 0 {
			level = gzip.DefaultCompression
		}
		return parquet.Compression(&gzip.Codec{Level: level}), nil
	case "zstd":
		if level == 0 {
			level = int(zstd.DefaultLevel)
		}
		return parquet.Compression(&zstd.Codec{Level: zstd.Level(level)}), nil
	case "lz4":
		if level == 0 {
			level = int(lz4.DefaultLevel)
		}
		return parquet.Compression(&lz4.Codec{Level: lz4.Level(level)}), nil
	case "snaapy":
		return parquet.Compression(&snappy.Codec{}), nil
	default:
		return nil, fmt.Errorf("unknown compression algorithm: %s", algo)
	}
}
