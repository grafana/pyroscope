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

const mappingsBlockHeaderSize = int(unsafe.Sizeof(mappingsBlockHeader{}))

var (
	_ symbolsBlockEncoder[v1.InMemoryMapping] = (*mappingsBlockEncoder)(nil)
	_ symbolsBlockDecoder[v1.InMemoryMapping] = (*mappingsBlockDecoder)(nil)
)

type mappingsBlockHeader struct {
	MappingsLen  uint32
	FileNameSize uint32
	BuildIDSize  uint32
	FlagsSize    uint32
	// Optional.
	MemoryStartSize uint32
	MemoryLimitSize uint32
	FileOffsetSize  uint32
}

func (h *mappingsBlockHeader) marshal(b []byte) {
	binary.LittleEndian.PutUint32(b[0:4], h.MappingsLen)
	binary.LittleEndian.PutUint32(b[4:8], h.FileNameSize)
	binary.LittleEndian.PutUint32(b[8:12], h.BuildIDSize)
	binary.LittleEndian.PutUint32(b[12:16], h.FlagsSize)
	binary.LittleEndian.PutUint32(b[16:20], h.MemoryStartSize)
	binary.LittleEndian.PutUint32(b[20:24], h.MemoryLimitSize)
	binary.LittleEndian.PutUint32(b[24:28], h.FileOffsetSize)
}

func (h *mappingsBlockHeader) unmarshal(b []byte) {
	h.MappingsLen = binary.LittleEndian.Uint32(b[0:4])
	h.FileNameSize = binary.LittleEndian.Uint32(b[4:8])
	h.BuildIDSize = binary.LittleEndian.Uint32(b[8:12])
	h.FlagsSize = binary.LittleEndian.Uint32(b[12:16])
	h.MemoryStartSize = binary.LittleEndian.Uint32(b[16:20])
	h.MemoryLimitSize = binary.LittleEndian.Uint32(b[20:24])
	h.FileOffsetSize = binary.LittleEndian.Uint32(b[24:28])
}

// isValid reports whether the header contains sane values.
// This is important as the block might be read before the
// checksum validation.
func (h *mappingsBlockHeader) isValid() bool {
	return h.MappingsLen < 1<<20
}

type mappingsBlockEncoder struct {
	header mappingsBlockHeader

	tmp    []byte
	buf    bytes.Buffer
	ints   []int32
	ints64 []int64
}

func (e *mappingsBlockEncoder) encode(w io.Writer, mappings []v1.InMemoryMapping) error {
	e.initWrite(len(mappings))
	var enc delta.BinaryPackedEncoding

	for i, m := range mappings {
		e.ints[i] = int32(m.Filename)
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.FileNameSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	for i, m := range mappings {
		e.ints[i] = int32(m.BuildId)
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.BuildIDSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	for i, m := range mappings {
		var v int32
		if m.HasFunctions {
			v |= 1 << 3
		}
		if m.HasFilenames {
			v |= 1 << 2
		}
		if m.HasLineNumbers {
			v |= 1 << 1
		}
		if m.HasInlineFrames {
			v |= 1
		}
		e.ints[i] = v
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.FlagsSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	var memoryStart uint64
	for i, m := range mappings {
		memoryStart |= m.MemoryStart
		e.ints64[i] = int64(m.MemoryStart)
	}
	if memoryStart != 0 {
		e.tmp, _ = enc.EncodeInt64(e.tmp, e.ints64)
		e.header.MemoryStartSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	var memoryLimit uint64
	for i, m := range mappings {
		memoryLimit |= m.MemoryLimit
		e.ints64[i] = int64(m.MemoryLimit)
	}
	if memoryLimit != 0 {
		e.tmp, _ = enc.EncodeInt64(e.tmp, e.ints64)
		e.header.MemoryLimitSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	var fileOffset uint64
	for i, m := range mappings {
		fileOffset |= m.FileOffset
		e.ints64[i] = int64(m.FileOffset)
	}
	if fileOffset != 0 {
		e.tmp, _ = enc.EncodeInt64(e.tmp, e.ints64)
		e.header.FileOffsetSize = uint32(len(e.tmp))
		e.buf.Write(e.tmp)
	}

	e.tmp = slices.GrowLen(e.tmp, mappingsBlockHeaderSize)
	e.header.marshal(e.tmp)
	if _, err := w.Write(e.tmp); err != nil {
		return err
	}
	_, err := e.buf.WriteTo(w)
	return err
}

func (e *mappingsBlockEncoder) initWrite(mappings int) {
	e.buf.Reset()
	// Actual estimate is ~7 bytes per mapping.
	e.buf.Grow(mappings * 8)
	*e = mappingsBlockEncoder{
		header: mappingsBlockHeader{MappingsLen: uint32(mappings)},

		tmp:    slices.GrowLen(e.tmp, mappings*2),
		ints:   slices.GrowLen(e.ints, mappings),
		ints64: slices.GrowLen(e.ints64, mappings),
		buf:    e.buf,
	}
}

type mappingsBlockDecoder struct {
	header mappingsBlockHeader

	ints   []int32
	ints64 []int64
	tmp    []byte
}

func (d *mappingsBlockDecoder) readHeader(r io.Reader) error {
	d.tmp = slices.GrowLen(d.tmp, mappingsBlockHeaderSize)
	if _, err := io.ReadFull(r, d.tmp); err != nil {
		return nil
	}
	d.header.unmarshal(d.tmp)
	if !d.header.isValid() {
		return ErrInvalidSize
	}
	// TODO: Scale tmp
	return nil
}

func (d *mappingsBlockDecoder) decode(r io.Reader, mappings []v1.InMemoryMapping) (err error) {
	if err = d.readHeader(r); err != nil {
		return err
	}
	if d.header.MappingsLen > uint32(len(mappings)) {
		return fmt.Errorf("mappings buffer is too short")
	}

	var enc delta.BinaryPackedEncoding
	d.ints = slices.GrowLen(d.ints, int(d.header.MappingsLen))

	d.tmp = slices.GrowLen(d.tmp, int(d.header.FileNameSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		mappings[i].Filename = uint32(v)
	}

	d.tmp = slices.GrowLen(d.tmp, int(d.header.BuildIDSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		mappings[i].BuildId = uint32(v)
	}

	d.tmp = slices.GrowLen(d.tmp, int(d.header.FlagsSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		mappings[i].HasFunctions = v&(1<<3) > 0
		mappings[i].HasFilenames = v&(1<<2) > 0
		mappings[i].HasLineNumbers = v&(1<<1) > 0
		mappings[i].HasInlineFrames = v&1 > 0
	}

	if d.header.MemoryStartSize > 0 {
		d.ints64 = slices.GrowLen(d.ints64, int(d.header.MappingsLen))
		d.tmp = slices.GrowLen(d.tmp, int(d.header.MemoryStartSize))
		if _, err = io.ReadFull(r, d.tmp); err != nil {
			return err
		}
		d.ints64, err = enc.DecodeInt64(d.ints64, d.tmp)
		if err != nil {
			return err
		}
		for i, v := range d.ints64 {
			mappings[i].MemoryStart = uint64(v)
		}
	}
	if d.header.MemoryLimitSize > 0 {
		d.ints64 = slices.GrowLen(d.ints64, int(d.header.MappingsLen))
		d.tmp = slices.GrowLen(d.tmp, int(d.header.MemoryLimitSize))
		if _, err = io.ReadFull(r, d.tmp); err != nil {
			return err
		}
		d.ints64, err = enc.DecodeInt64(d.ints64, d.tmp)
		if err != nil {
			return err
		}
		for i, v := range d.ints64 {
			mappings[i].MemoryLimit = uint64(v)
		}
	}
	if d.header.FileOffsetSize > 0 {
		d.ints64 = slices.GrowLen(d.ints64, int(d.header.MappingsLen))
		d.tmp = slices.GrowLen(d.tmp, int(d.header.FileOffsetSize))
		if _, err = io.ReadFull(r, d.tmp); err != nil {
			return err
		}
		d.ints64, err = enc.DecodeInt64(d.ints64, d.tmp)
		if err != nil {
			return err
		}
		for i, v := range d.ints64 {
			mappings[i].FileOffset = uint64(v)
		}
	}

	return nil
}
