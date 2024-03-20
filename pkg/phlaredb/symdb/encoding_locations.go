package symdb

import (
	"bytes"
	"encoding/binary"
	"io"
	"unsafe"

	"github.com/parquet-go/parquet-go/encoding/delta"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

// https://parquet.apache.org/docs/file-format/data-pages/encodings/#delta-encoding-delta_binary_packed--5

type LocationsEncoder struct {
	w io.Writer
}

type locationsBlock struct {
	locsLen uint32

	mapping  []int32
	function []int32
	line     []int32
	// Optional.
	count  []int32
	lines  []int32
	addr   []int64
	folded []bool

	tmp []byte
	buf bytes.Buffer

	hasLines  bool
	hasAddr   bool
	hasFolded bool
}

func (lb *locationsBlock) encode(w io.Writer, locations []v1.InMemoryLocation) (int64, error) {
	lb.reset(len(locations))
	var addr int64
	for i, loc := range locations {
		lb.mapping[i] = int32(loc.MappingId)
		lb.function[i] = int32(loc.Line[0].FunctionId)
		lb.line[i] = loc.Line[0].Line
		lb.count[i] = int32(len(loc.Line) - 1)
		// Append lines but the first one.
		for j := 1; j < len(loc.Line); j++ {
			line := loc.Line[j]
			lb.lines = append(lb.lines, line.Line, int32(line.FunctionId))
		}
		addr |= int64(loc.Address)
		lb.addr[i] = int64(loc.Address)
		lb.hasFolded = lb.hasFolded || loc.IsFolded
		lb.folded[i] = loc.IsFolded
	}
	lb.hasLines = len(lb.lines) > 0
	lb.hasAddr = addr > 0
	h := locationsBlockHeader{
		LocationsLen: lb.locsLen,
	}

	var enc delta.BinaryPackedEncoding
	lb.tmp, _ = enc.EncodeInt32(lb.tmp, lb.mapping)
	h.MappingSize = uint32(len(lb.tmp))
	lb.buf.Write(lb.tmp)
	lb.tmp, _ = enc.EncodeInt32(lb.tmp, lb.function)
	h.FunctionSize = uint32(len(lb.tmp))
	lb.buf.Write(lb.tmp)
	lb.tmp, _ = enc.EncodeInt32(lb.tmp, lb.line)
	h.LineSize = uint32(len(lb.tmp))
	lb.buf.Write(lb.tmp)
	if lb.hasLines {
		lb.tmp, _ = enc.EncodeInt32(lb.tmp, lb.count)
		h.CountSize = uint32(len(lb.tmp))
		lb.buf.Write(lb.tmp)
		lb.tmp, _ = enc.EncodeInt32(lb.tmp, lb.lines)
		h.LinesSize = uint32(len(lb.tmp))
		lb.buf.Write(lb.tmp)
	}
	if lb.hasAddr {
		lb.tmp, _ = enc.EncodeInt64(lb.tmp, lb.addr)
		h.AddrSize = uint32(len(lb.tmp))
		lb.buf.Write(lb.tmp)
	}
	if lb.hasFolded {
		// TODO
	}

	lb.tmp = slices.GrowLen(lb.tmp, locationsBlockHeaderSize)
	h.marshal(lb.tmp)
	n, err := w.Write(lb.tmp)
	if err != nil {
		return int64(n), err
	}
	m, err := lb.buf.WriteTo(w)
	return m + int64(n), err
}

func (lb *locationsBlock) reset(locations int) {
	// Actual estimate is ~6 bytes per location.
	// In a large data set, the most expensive member
	// is FunctionID, and it's about 2 bytes per location.
	lb.buf.Reset()
	lb.buf.Grow(locations * 8)
	*lb = locationsBlock{
		locsLen: uint32(locations),

		mapping:  slices.GrowLen(lb.mapping, locations),
		function: slices.GrowLen(lb.function, locations),
		line:     slices.GrowLen(lb.line, locations),

		count:  slices.GrowLen(lb.count, locations),
		lines:  lb.lines[:0], // Appended.
		addr:   slices.GrowLen(lb.addr, locations),
		folded: slices.GrowLen(lb.folded, locations),

		buf: lb.buf,
		tmp: slices.GrowLen(lb.tmp, 2*locations),
	}
}

const locationsBlockHeaderSize = int(unsafe.Sizeof(locationsBlockHeader{}))

type locationsBlockHeader struct {
	LocationsLen uint32
	MappingSize  uint32
	FunctionSize uint32
	LineSize     uint32
	CountSize    uint32
	LinesSize    uint32
	AddrSize     uint32
	IsFoldedSize uint32
}

func (h *locationsBlockHeader) marshal(b []byte) {
	binary.LittleEndian.PutUint32(b[0:4], h.LocationsLen)
	binary.LittleEndian.PutUint32(b[4:8], h.MappingSize)
	binary.LittleEndian.PutUint32(b[8:12], h.FunctionSize)
	binary.LittleEndian.PutUint32(b[12:16], h.LineSize)
	binary.LittleEndian.PutUint32(b[16:20], h.CountSize)
	binary.LittleEndian.PutUint32(b[20:24], h.LinesSize)
	binary.LittleEndian.PutUint32(b[24:28], h.AddrSize)
	binary.LittleEndian.PutUint32(b[28:32], h.IsFoldedSize)
}

func (lb *locationsBlock) locations() int { return int(lb.locsLen) }

func (lb *locationsBlock) decode(locations []v1.InMemoryLocation) {
	lines := make([]v1.InMemoryLine, len(lb.function)+len(lb.lines)/2)
	var j int32 // Offset within the lines slice.
	var o int32 // Offset within the encoded lines slice.
	for i := 0; i < len(locations); i++ {
		ll := lb.count[i] + 1
		locations[i].Line = lines[j : j+ll]
		locations[i].Line[0].Line = lb.line[i]
		locations[i].Line[0].FunctionId = uint32(lb.function[i])
		locations[i].MappingId = uint32(lb.mapping[i])
		j += ll
		for l := int32(1); l < ll; l++ {
			locations[i].Line[l].FunctionId = uint32(lb.lines[o+1])
			locations[i].Line[l].Line = lb.lines[o]
			o += 2
		}
	}
}

func NewLocationsEncoder(w io.Writer) *LocationsEncoder {
	return &LocationsEncoder{w: w}
}

func (e *LocationsEncoder) EncodeLocations(locations []v1.InMemoryLocation) error {
	return nil
}
