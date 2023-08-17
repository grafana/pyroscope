package parser

import (
	"github.com/pyroscope-io/jfr-parser/reader"
)

const headerSize = 60

type Header struct {
	ChunkSize          int64
	ConstantPoolOffset int64
	MetadataOffset     int64
	StartTimeNanos     int64
	DurationNanos      int64
	StartTicks         int64
	TicksPerSecond     int64
	Features           int32
}

func (h *Header) Parse(rd reader.Reader) (err error) {
	h.ChunkSize, _ = rd.Long()
	h.ConstantPoolOffset, _ = rd.Long()
	h.MetadataOffset, _ = rd.Long()
	h.StartTimeNanos, _ = rd.Long()
	h.DurationNanos, _ = rd.Long()
	h.StartTicks, _ = rd.Long()
	h.TicksPerSecond, _ = rd.Long()
	h.Features, err = rd.Int()
	return err
}
