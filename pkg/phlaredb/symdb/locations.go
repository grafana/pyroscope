package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"

	"github.com/parquet-go/parquet-go/encoding/delta"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

const (
	maxLocationLines         = 255
	locationsBlockHeaderSize = int(unsafe.Sizeof(locationsBlockHeader{}))
)

var (
	_ symbolsBlockEncoder[v1.InMemoryLocation] = (*locationsBlockEncoder)(nil)
	_ symbolsBlockDecoder[v1.InMemoryLocation] = (*locationsBlockDecoder)(nil)
)

type locationsBlockHeader struct {
	LocationsLen uint32 // Number of locations
	MappingSize  uint32 // Size of the encoded slice of mapping_ids
	LinesLen     uint32 // Number of lines per location
	LinesSize    uint32 // Size of the encoded lines
	// Optional, might be empty.
	AddrSize     uint32 // Size of the encoded slice of addresses
	IsFoldedSize uint32 // Size of the encoded slice of is_folded
}

func (h *locationsBlockHeader) isValid() bool {
	return h.LocationsLen > 0 && h.LocationsLen < 1<<20 &&
		h.MappingSize > 0 && h.MappingSize < 1<<20 &&
		h.LinesLen > 0 && h.LinesLen < 1<<20 &&
		h.LinesSize > 0 && h.LinesSize < 1<<20 &&
		h.AddrSize < 1<<20 &&
		h.IsFoldedSize < 1<<20
}

func (h *locationsBlockHeader) marshal(b []byte) {
	binary.LittleEndian.PutUint32(b[0:4], h.LocationsLen)
	binary.LittleEndian.PutUint32(b[4:8], h.MappingSize)
	binary.LittleEndian.PutUint32(b[8:12], h.LinesLen)
	binary.LittleEndian.PutUint32(b[12:16], h.LinesSize)
	binary.LittleEndian.PutUint32(b[16:20], h.AddrSize)
	binary.LittleEndian.PutUint32(b[20:24], h.IsFoldedSize)
}

func (h *locationsBlockHeader) unmarshal(b []byte) {
	h.LocationsLen = binary.LittleEndian.Uint32(b[0:4])
	h.MappingSize = binary.LittleEndian.Uint32(b[4:8])
	h.LinesLen = binary.LittleEndian.Uint32(b[8:12])
	h.LinesSize = binary.LittleEndian.Uint32(b[12:16])
	h.AddrSize = binary.LittleEndian.Uint32(b[16:20])
	h.IsFoldedSize = binary.LittleEndian.Uint32(b[20:24])
}

type locationsBlockEncoder struct {
	header locationsBlockHeader

	mapping []int32
	// Assuming there is no locations with more than 255 lines.
	// We could even use a nibble (4 bits), but there are locations
	// with 10 and more functions, therefore there is a change that
	// capacity of 2^4 is not enough in all cases.
	lineCount []byte
	lines     []int32
	// Optional.
	addr   []int64
	folded []bool

	tmp []byte
	buf bytes.Buffer
}

func (e *locationsBlockEncoder) encode(w io.Writer, locations []v1.InMemoryLocation) error {
	e.initWrite(len(locations))
	var addr int64
	var folded bool
	for i, loc := range locations {
		e.mapping[i] = int32(loc.MappingId)
		e.lineCount[i] = byte(len(loc.Line))
		// Append lines but the first one.
		for j := 0; j < len(loc.Line) && j < maxLocationLines; j++ {
			e.lines = append(e.lines,
				int32(loc.Line[j].FunctionId),
				loc.Line[j].Line)
		}
		addr |= int64(loc.Address)
		e.addr[i] = int64(loc.Address)
		folded = folded || loc.IsFolded
		e.folded[i] = loc.IsFolded
	}

	// Mapping and line count per location.
	var enc delta.BinaryPackedEncoding
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.mapping)
	e.header.MappingSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)
	// Line count size and length is deterministic.
	e.buf.Write(e.lineCount) // Without any encoding.

	// Lines slice size and length (in lines, not int32s).
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.lines)
	e.header.LinesLen = uint32(len(e.lines) / 2)
	e.header.LinesSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	if addr > 0 {
		e.tmp, _ = enc.EncodeInt64(e.tmp, e.addr)
		e.header.AddrSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	if folded {
		e.tmp = slices.GrowLen(e.tmp, len(e.folded)/8)
		encodeBoolean(e.tmp, e.folded)
		e.header.IsFoldedSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	e.tmp = slices.GrowLen(e.tmp, locationsBlockHeaderSize)
	e.header.marshal(e.tmp)
	if _, err := w.Write(e.tmp); err != nil {
		return err
	}
	_, err := e.buf.WriteTo(w)
	return err
}

func (e *locationsBlockEncoder) initWrite(locations int) {
	// Actual estimate is ~6 bytes per location.
	// In a large data set, the most expensive member
	// is FunctionID, and it's about 2 bytes per location.
	e.buf.Reset()
	e.buf.Grow(locations * 8)
	*e = locationsBlockEncoder{
		header: locationsBlockHeader{LocationsLen: uint32(locations)},

		mapping:   slices.GrowLen(e.mapping, locations),
		lineCount: slices.GrowLen(e.lineCount, locations),
		lines:     e.lines[:0], // Appendable.
		addr:      slices.GrowLen(e.addr, locations),
		folded:    slices.GrowLen(e.folded, locations),

		buf: e.buf,
		tmp: slices.GrowLen(e.tmp, 2*locations),
	}
}

type locationsBlockDecoder struct {
	header locationsBlockHeader

	mappings  []int32
	lineCount []byte
	lines     []int32

	address []int64
	folded  []bool

	tmp []byte
}

func (d *locationsBlockDecoder) readHeader(r io.Reader) error {
	d.tmp = slices.GrowLen(d.tmp, locationsBlockHeaderSize)
	if _, err := io.ReadFull(r, d.tmp); err != nil {
		return nil
	}
	d.header.unmarshal(d.tmp)
	if !d.header.isValid() {
		return ErrInvalidSize
	}
	return nil
}

func (d *locationsBlockDecoder) decode(r io.Reader, locations []v1.InMemoryLocation) (err error) {
	if err = d.readHeader(r); err != nil {
		return err
	}
	if d.header.LocationsLen > uint32(len(locations)) {
		return fmt.Errorf("locations buffer is too short")
	}

	var enc delta.BinaryPackedEncoding
	// First we decode mapping_id and assign them to locations.
	d.tmp = slices.GrowLen(d.tmp, int(d.header.MappingSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.mappings, err = enc.DecodeInt32(d.mappings, d.tmp)
	if err != nil {
		return err
	}

	// Line count per location.
	// One byte per location.
	d.lineCount = slices.GrowLen(d.lineCount, int(d.header.LocationsLen))
	if _, err = io.ReadFull(r, d.lineCount); err != nil {
		return err
	}

	// Lines. A single slice backs all the location line
	// sub-slices. But it has to be allocated as we can't
	// reference d.lines, which is reusable.
	lines := make([]v1.InMemoryLine, d.header.LinesLen)
	d.tmp = slices.GrowLen(d.tmp, int(d.header.LinesSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.lines = slices.GrowLen(d.lines, int(d.header.LinesLen))
	d.lines, err = enc.DecodeInt32(d.lines, d.tmp)
	if err != nil {
		return err
	}
	copy(lines, *(*[]v1.InMemoryLine)(unsafe.Pointer(&d.lines)))

	// In most cases we end up here.
	if d.header.AddrSize == 0 && d.header.IsFoldedSize == 0 {
		var o int // Offset within the lines slice.
		for i := 0; i < len(locations); i++ {
			locations[i].MappingId = uint32(d.mappings[i])
			n := o + int(d.lineCount[i])
			locations[i].Line = lines[o:n]
			o = n
		}
		return nil
	}

	// Otherwise, inspect all the optional fields.
	if int(d.header.AddrSize) > 0 {
		d.tmp = slices.GrowLen(d.tmp, int(d.header.AddrSize))
		if _, err = io.ReadFull(r, d.tmp); err != nil {
			return err
		}
		d.address = slices.GrowLen(d.address, int(d.header.LocationsLen))
		d.address, err = enc.DecodeInt64(d.address, d.tmp)
		if err != nil {
			return err
		}
	}
	if int(d.header.IsFoldedSize) > 0 {
		d.tmp = slices.GrowLen(d.tmp, int(d.header.IsFoldedSize))
		if _, err = io.ReadFull(r, d.tmp); err != nil {
			return err
		}
		d.folded = slices.GrowLen(d.folded, int(d.header.LocationsLen))
		decodeBoolean(d.folded, d.tmp)
	}

	var o int // Offset within the lines slice.
	for i := uint32(0); i < d.header.LocationsLen; i++ {
		locations[i].MappingId = uint32(d.mappings[i])
		n := o + int(d.lineCount[i])
		locations[i].Line = lines[o:n]
		o = n
		locations[i].Address = uint64(d.address[i])
		locations[i].IsFolded = d.folded[i]
	}

	return nil
}

func encodeBoolean(dst []byte, src []bool) {
	for i := range dst {
		dst[i] = 0
	}
	for i, b := range src {
		if b {
			dst[i>>3] |= 1 << i & 7
		}
	}
}

func decodeBoolean(dst []bool, src []byte) {
	for i := range dst {
		dst[i] = false
	}
	for i := range dst {
		dst[i] = src[i>>3]&(1<<i&7) != 0
	}
}
