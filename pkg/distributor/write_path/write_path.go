package writepath

import (
	"errors"
	"flag"
	"fmt"
	"time"
)

// WritePath controls the write path.
type WritePath string

const (
	// IngesterPath specifies old write path the requests are sent to ingester.
	IngesterPath WritePath = "ingester"
	// SegmentWriterPath specifies the new write path: distributor sends
	// the request to segment writers before profile split, using the new
	// distribution algorithm and the segment-writer ring.
	SegmentWriterPath = "segment-writer"
	// CombinedPath specifies that each request should be sent to both write
	// paths. For each request we decide on how a failure is handled:
	//  * If the request is sent to ingester (regardless of anything),
	//    the response is returned to the client immediately after the old
	//    write path returns. Failure of the new write path should be logged
	//    and counted in metrics but NOT returned to the client.
	//  * If the request is sent to segment-writer exclusively: the response
	//    returns to the client only when the new write path returns.
	//    Failure of the new write is returned to the client.
	//    Failure of the old write path is NOT returned to the client.
	CombinedPath = "combined"
)

var ErrInvalidWritePath = errors.New("invalid write path")

var paths = []WritePath{
	IngesterPath,
	SegmentWriterPath,
	CombinedPath,
}

const validWritePathOptionsString = "valid options: ingester, segment-writer, combined"

func (m *WritePath) Set(text string) error {
	x := WritePath(text)
	for _, name := range paths {
		if x == name {
			*m = x
			return nil
		}
	}
	return fmt.Errorf("%w: %s; %s", ErrInvalidWritePath, x, validWritePathOptionsString)
}

func (m *WritePath) String() string { return string(*m) }

type Compression string

const (
	CompressionNone Compression = "none"
	CompressionGzip Compression = "gzip"
)

var ErrInvalidCompression = errors.New("invalid write path compression")

var compressions = []Compression{
	CompressionNone,
	CompressionGzip,
}

const validCompressionOptionsString = "valid compression options: none, gzip"

func (m *Compression) Set(text string) error {
	x := Compression(text)
	for _, name := range compressions {
		if x == name {
			*m = x
			return nil
		}
	}
	return fmt.Errorf("%w: %s; %s", ErrInvalidCompression, x, validCompressionOptionsString)
}

func (m *Compression) String() string { return string(*m) }

type Config struct {
	WritePath            WritePath     `yaml:"write_path" json:"write_path" doc:"hidden"`
	IngesterWeight       float64       `yaml:"write_path_ingester_weight" json:"write_path_ingester_weight" doc:"hidden"`
	SegmentWriterWeight  float64       `yaml:"write_path_segment_writer_weight" json:"write_path_segment_writer_weight" doc:"hidden"`
	SegmentWriterTimeout time.Duration `yaml:"write_path_segment_writer_timeout" json:"write_path_segment_writer_timeout" doc:"hidden"`
	Compression          Compression   `yaml:"write_path_compression" json:"write_path_compression" doc:"hidden"`
	AsyncIngest          bool          `yaml:"async_ingest" json:"async_ingest" doc:"hidden"`
}

func (o *Config) RegisterFlags(f *flag.FlagSet) {
	o.WritePath = IngesterPath
	o.Compression = CompressionNone
	f.Var(&o.WritePath, "write-path", "Controls the write path route; "+validWritePathOptionsString+".")
	f.Float64Var(&o.IngesterWeight, "write-path.ingester-weight", 1,
		"Specifies the fraction [0:1] that should be send to ingester in combined mode. 0 means no traffics is sent to ingester. 1 means 100% of requests are sent to ingester.")
	f.Float64Var(&o.SegmentWriterWeight, "write-path.segment-writer-weight", 0,
		"Specifies the fraction [0:1] that should be send to segment-writer in combined mode. 0 means no traffics is sent to segment-writer. 1 means 100% of requests are sent to segment-writer.")
	f.DurationVar(&o.SegmentWriterTimeout, "write-path.segment-writer-timeout", 5*time.Second, "Timeout for segment writer requests.")
	f.Var(&o.Compression, "write-path.compression", "Compression algorithm to use for segment writer requests; "+validCompressionOptionsString+".")
	f.BoolVar(&o.AsyncIngest, "async-ingest", false, "If true, the write path will not wait for the segment-writer to finish processing the request. Writes to ingester always synchronous.")
}
