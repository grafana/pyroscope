package gzipwriter

import (
	"flag"
	"fmt"

	"go.uber.org/atomic"
)

const DefaultParallelMinSize = 1 << 20

var parallelMinSize = atomic.NewInt64(DefaultParallelMinSize)

type Config struct {
	ParallelGzipMinSize int64 `yaml:"parallel_gzip_min_size"`
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	f.Int64Var(&c.ParallelGzipMinSize, "compression.parallel-gzip-min-size", DefaultParallelMinSize,
		"Minimum uncompressed message size in bytes for parallel gzip in internal pprof serialization and Connect RPC compression. Smaller messages use serial gzip. Zero always uses parallel gzip.")
}

func (c Config) Validate() error {
	if c.ParallelGzipMinSize < 0 {
		return fmt.Errorf("compression.parallel-gzip-min-size must not be negative")
	}
	return nil
}

// Configure sets the process-wide compression policy at application startup.
// Pooled adaptive writers consult it when starting each stream. An active stream
// keeps its selected compressor until Reset.
func Configure(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	parallelMinSize.Store(c.ParallelGzipMinSize)
	return nil
}
