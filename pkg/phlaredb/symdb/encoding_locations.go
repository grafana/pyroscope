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
	"github.com/grafana/pyroscope/pkg/util/math"
)

// https://parquet.apache.org/docs/file-format/data-pages/encodings/#delta-encoding-delta_binary_packed--5

type LocationsEncoder struct {
	w io.Writer
	e locationsBlockEncoder

	blockSize int
	locations int

	buf []byte
}

const (
	maxLocationLines          = 255
	defaultLocationsBlockSize = 1 << 10
)

func NewLocationsEncoder(w io.Writer) *LocationsEncoder {
	return &LocationsEncoder{w: w}
}

func (e *LocationsEncoder) EncodeLocations(locations []v1.InMemoryLocation) error {
	if e.blockSize == 0 {
		e.blockSize = defaultLocationsBlockSize
	}
	e.locations = len(locations)
	if err := e.writeHeader(); err != nil {
		return err
	}
	for i := 0; i < len(locations); i += e.blockSize {
		block := locations[i:math.Min(i+e.blockSize, len(locations))]
		if _, err := e.e.encode(e.w, block); err != nil {
			return err
		}
	}
	return nil
}

func (e *LocationsEncoder) writeHeader() (err error) {
	e.buf = slices.GrowLen(e.buf, 8)
	binary.LittleEndian.PutUint32(e.buf[0:4], uint32(e.locations))
	binary.LittleEndian.PutUint32(e.buf[4:8], uint32(e.blockSize))
	_, err = e.w.Write(e.buf)
	return err
}

func (e *LocationsEncoder) Reset(w io.Writer) {
	e.locations = 0
	e.blockSize = 0
	e.buf = e.buf[:0]
	e.w = w
}

type LocationsDecoder struct {
	r io.Reader
	d locationsBlockDecoder

	blockSize uint32
	locations uint32

	buf []byte
}

func NewLocationsDecoder(r io.Reader) *LocationsDecoder { return &LocationsDecoder{r: r} }

func (d *LocationsDecoder) LocationsLen() (int, error) {
	if err := d.readHeader(); err != nil {
		return 0, err
	}
	return int(d.locations), nil
}

func (d *LocationsDecoder) readHeader() (err error) {
	d.buf = slices.GrowLen(d.buf, 8)
	if _, err = io.ReadFull(d.r, d.buf); err != nil {
		return err
	}
	d.locations = binary.LittleEndian.Uint32(d.buf[0:4])
	d.blockSize = binary.LittleEndian.Uint32(d.buf[4:8])
	// Sanity checks are needed as we process the stream data
	// before verifying the check sum.
	if d.locations > 1<<20 || d.blockSize > 1<<20 {
		return ErrInvalidSize
	}
	return nil
}

func (d *LocationsDecoder) DecodeLocations(locations []v1.InMemoryLocation) error {
	blocks := int((d.locations + d.blockSize - 1) / d.blockSize)
	for i := 0; i < blocks; i++ {
		lo := i * int(d.blockSize)
		hi := math.Min(lo+int(d.blockSize), int(d.locations))
		block := locations[lo:hi]
		if err := d.d.decode(d.r, block); err != nil {
			return err
		}
	}
	return nil
}

func (d *LocationsDecoder) Reset(r io.Reader) {
	d.locations = 0
	d.blockSize = 0
	d.buf = d.buf[:0]
	d.r = r
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

	hasFolded bool
}

const locationsBlockHeaderSize = int(unsafe.Sizeof(locationsBlockHeader{}))

type locationsBlockHeader struct {
	LocationsLen uint32 // Number of locations
	MappingSize  uint32 // Size of the encoded slice of mapping_ids
	LinesLen     uint32 // Number of lines per location
	LinesSize    uint32 // Size of the encoded lines
	// Optional, might be empty.
	AddrSize     uint32 // Size of the encoded slice of addresses
	IsFoldedSize uint32 // Size of the encoded slice of is_folded
}

// isValid reports whether the header contains sane values.
// This is important as the block might be read before the
// checksum validation.
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

func (e *locationsBlockEncoder) encode(w io.Writer, locations []v1.InMemoryLocation) (int64, error) {
	e.initWrite(len(locations))
	var addr int64
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
		e.hasFolded = e.hasFolded || loc.IsFolded
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

	if e.hasFolded {
		e.tmp = slices.GrowLen(e.tmp, len(e.folded)/8)
		encodeBoolean(e.tmp, e.folded)
		e.header.IsFoldedSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	e.tmp = slices.GrowLen(e.tmp, locationsBlockHeaderSize)
	e.header.marshal(e.tmp)
	n, err := w.Write(e.tmp)
	if err != nil {
		return int64(n), err
	}
	m, err := e.buf.WriteTo(w)
	return m + int64(n), err
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
