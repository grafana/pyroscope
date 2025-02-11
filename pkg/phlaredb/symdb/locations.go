//nolint:unused
package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"unsafe"

	"github.com/parquet-go/parquet-go/encoding/delta"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

const maxLocationLines = 255

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
	CRC          uint32 // Header CRC.
}

func (h *locationsBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint32(b[0:4], h.LocationsLen)
	binary.BigEndian.PutUint32(b[4:8], h.MappingSize)
	binary.BigEndian.PutUint32(b[8:12], h.LinesLen)
	binary.BigEndian.PutUint32(b[12:16], h.LinesSize)
	binary.BigEndian.PutUint32(b[16:20], h.AddrSize)
	binary.BigEndian.PutUint32(b[20:24], h.IsFoldedSize)
	// Fields can be added here in the future.
	// CRC must be the last four bytes.
	h.CRC = crc32.Checksum(b[0:24], castagnoli)
	binary.BigEndian.PutUint32(b[24:28], h.CRC)
}

func (h *locationsBlockHeader) unmarshal(b []byte) {
	h.LocationsLen = binary.BigEndian.Uint32(b[0:4])
	h.MappingSize = binary.BigEndian.Uint32(b[4:8])
	h.LinesLen = binary.BigEndian.Uint32(b[8:12])
	h.LinesSize = binary.BigEndian.Uint32(b[12:16])
	h.AddrSize = binary.BigEndian.Uint32(b[16:20])
	h.IsFoldedSize = binary.BigEndian.Uint32(b[20:24])
	// In future versions, new fields are decoded here;
	// if pos < len(b)-checksumSize, then there are more fields.
	h.CRC = binary.BigEndian.Uint32(b[24:28])
}

func (h *locationsBlockHeader) checksum() uint32 { return h.CRC }

type locationsBlockEncoder struct {
	header locationsBlockHeader

	mapping []int32
	// Assuming there are no locations with more than 255 lines.
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

func newLocationsEncoder() *symbolsEncoder[v1.InMemoryLocation] {
	return newSymbolsEncoder[v1.InMemoryLocation](new(locationsBlockEncoder))
}

func (e *locationsBlockEncoder) format() SymbolsBlockFormat { return BlockLocationsV1 }

func (e *locationsBlockEncoder) headerSize() uintptr { return unsafe.Sizeof(locationsBlockHeader{}) }

func (e *locationsBlockEncoder) encode(w io.Writer, locations []v1.InMemoryLocation) error {
	e.initWrite(len(locations))
	var addr uint64
	var folded bool
	for i, loc := range locations {
		e.mapping[i] = int32(loc.MappingId)
		e.lineCount[i] = byte(len(loc.Line))
		for j := 0; j < len(loc.Line) && j < maxLocationLines; j++ {
			e.lines = append(e.lines,
				int32(loc.Line[j].FunctionId),
				loc.Line[j].Line)
		}
		addr |= loc.Address
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
		e.tmp = slices.GrowLen(e.tmp, len(e.folded)/8+1)
		encodeBoolean(e.tmp, e.folded)
		e.header.IsFoldedSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	e.tmp = slices.GrowLen(e.tmp, int(e.headerSize()))
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
	headerSize uint16
	header     locationsBlockHeader

	mappings  []int32
	lineCount []byte
	lines     []int32

	address []int64
	folded  []bool

	buf []byte
}

func newLocationsDecoder(h SymbolsBlockHeader) (*symbolsDecoder[v1.InMemoryLocation], error) {
	if h.Format == BlockLocationsV1 {
		headerSize := max(locationsBlockHeaderMinSize, h.BlockHeaderSize)
		return newSymbolsDecoder[v1.InMemoryLocation](h, &locationsBlockDecoder{headerSize: headerSize}), nil
	}
	return nil, fmt.Errorf("%w: unknown locations format: %d", ErrUnknownVersion, h.Format)
}

// In early versions, block header size is not specified. Must not change.
const locationsBlockHeaderMinSize = 28

func (d *locationsBlockDecoder) decode(r io.Reader, locations []v1.InMemoryLocation) (err error) {
	d.buf = slices.GrowLen(d.buf, int(d.headerSize))
	if err = readSymbolsBlockHeader(d.buf, r, &d.header); err != nil {
		return err
	}
	if d.header.LocationsLen != uint32(len(locations)) {
		return fmt.Errorf("locations buffer: %w", ErrInvalidSize)
	}

	// First we decode mapping_id and assign them to locations.
	d.buf = slices.GrowLen(d.buf, int(d.header.MappingSize))
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	d.mappings, err = decodeBinaryPackedInt32(d.mappings, d.buf, int(d.header.LocationsLen))
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
	d.buf = slices.GrowLen(d.buf, int(d.header.LinesSize))
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	// Lines are encoded as pairs of uint32 (function_id and line number).
	d.lines, err = decodeBinaryPackedInt32(d.lines, d.buf, int(d.header.LinesLen)*2)
	if err != nil {
		return err
	}
	copy(lines, *(*[]v1.InMemoryLine)(unsafe.Pointer(&d.lines)))

	// In most cases we end up here.
	if d.header.AddrSize == 0 && d.header.IsFoldedSize == 0 {
		var o int // Offset within the lines slice.
		// In case if the block is malformed, an invalid
		// line count may cause an out-of-bounds panic.
		maxLines := len(lines)
		for i := 0; i < len(locations); i++ {
			locations[i].MappingId = uint32(d.mappings[i])
			n := o + int(d.lineCount[i])
			if n > maxLines {
				return fmt.Errorf("%w: location lines out of bounds", ErrInvalidSize)
			}
			locations[i].Line = lines[o:n]
			o = n
		}
		return nil
	}

	// Otherwise, inspect all the optional fields.
	d.address = slices.GrowLen(d.address, int(d.header.LocationsLen))
	d.folded = slices.GrowLen(d.folded, int(d.header.LocationsLen))
	if int(d.header.AddrSize) > 0 {
		d.buf = slices.GrowLen(d.buf, int(d.header.AddrSize))
		if _, err = io.ReadFull(r, d.buf); err != nil {
			return err
		}
		d.address, err = decodeBinaryPackedInt64(d.address, d.buf, int(d.header.LocationsLen))
		if err != nil {
			return err
		}
	}
	if int(d.header.IsFoldedSize) > 0 {
		d.buf = slices.GrowLen(d.buf, int(d.header.IsFoldedSize))
		if _, err = io.ReadFull(r, d.buf); err != nil {
			return err
		}
		decodeBoolean(d.folded, d.buf)
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
