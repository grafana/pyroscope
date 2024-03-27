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

const functionsBlockHeaderSize = int(unsafe.Sizeof(functionsBlockHeader{}))

var (
	_ symbolsBlockEncoder[v1.InMemoryFunction] = (*functionsBlockEncoder)(nil)
	_ symbolsBlockDecoder[v1.InMemoryFunction] = (*functionsBlockDecoder)(nil)
)

type functionsBlockHeader struct {
	FunctionsLen   uint32
	NameSize       uint32
	SystemNameSize uint32
	FileNameSize   uint32
	StartLineSize  uint32
}

func (h *functionsBlockHeader) marshal(b []byte) {
	binary.LittleEndian.PutUint32(b[0:4], h.FunctionsLen)
	binary.LittleEndian.PutUint32(b[4:8], h.NameSize)
	binary.LittleEndian.PutUint32(b[8:12], h.SystemNameSize)
	binary.LittleEndian.PutUint32(b[12:16], h.FileNameSize)
	binary.LittleEndian.PutUint32(b[16:20], h.StartLineSize)
}

func (h *functionsBlockHeader) unmarshal(b []byte) {
	h.FunctionsLen = binary.LittleEndian.Uint32(b[0:4])
	h.NameSize = binary.LittleEndian.Uint32(b[4:8])
	h.SystemNameSize = binary.LittleEndian.Uint32(b[8:12])
	h.FileNameSize = binary.LittleEndian.Uint32(b[12:16])
	h.StartLineSize = binary.LittleEndian.Uint32(b[16:20])
}

// isValid reports whether the header contains sane values.
// This is important as the block might be read before the
// checksum validation.
func (h *functionsBlockHeader) isValid() bool {
	return h.FunctionsLen < 1<<20
}

type functionsBlockEncoder struct {
	header functionsBlockHeader

	tmp  []byte
	buf  bytes.Buffer
	ints []int32
}

func (e *functionsBlockEncoder) encode(w io.Writer, functions []v1.InMemoryFunction) error {
	e.initWrite(len(functions))
	var enc delta.BinaryPackedEncoding

	for i, f := range functions {
		e.ints[i] = int32(f.Name)
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.NameSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	for i, f := range functions {
		e.ints[i] = int32(f.SystemName)
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.SystemNameSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	for i, f := range functions {
		e.ints[i] = int32(f.Filename)
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.FileNameSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	for i, f := range functions {
		e.ints[i] = int32(f.StartLine)
	}
	e.tmp, _ = enc.EncodeInt32(e.tmp, e.ints)
	e.header.StartLineSize = uint32(len(e.tmp))
	e.buf.Write(e.tmp)

	e.tmp = slices.GrowLen(e.tmp, functionsBlockHeaderSize)
	e.header.marshal(e.tmp)
	if _, err := w.Write(e.tmp); err != nil {
		return err
	}
	_, err := e.buf.WriteTo(w)
	return err
}

func (e *functionsBlockEncoder) initWrite(functions int) {
	e.buf.Reset()
	// Actual estimate is ~7 bytes per function.
	e.buf.Grow(functions * 8)
	*e = functionsBlockEncoder{
		header: functionsBlockHeader{FunctionsLen: uint32(functions)},

		tmp:  slices.GrowLen(e.tmp, functions*2),
		ints: slices.GrowLen(e.ints, functions),
		buf:  e.buf,
	}
}

type functionsBlockDecoder struct {
	header functionsBlockHeader

	ints []int32
	tmp  []byte
}

func (d *functionsBlockDecoder) readHeader(r io.Reader) error {
	d.tmp = slices.GrowLen(d.tmp, functionsBlockHeaderSize)
	if _, err := io.ReadFull(r, d.tmp); err != nil {
		return nil
	}
	d.header.unmarshal(d.tmp)
	if !d.header.isValid() {
		return ErrInvalidSize
	}
	return nil
}

func (d *functionsBlockDecoder) decode(r io.Reader, functions []v1.InMemoryFunction) (err error) {
	if err = d.readHeader(r); err != nil {
		return err
	}
	if d.header.FunctionsLen > uint32(len(functions)) {
		return fmt.Errorf("functions buffer is too short")
	}

	var enc delta.BinaryPackedEncoding
	d.ints = slices.GrowLen(d.ints, int(d.header.FunctionsLen))
	d.tmp = slices.GrowLen(d.tmp, int(d.header.NameSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].Name = uint32(v)
	}

	d.tmp = slices.GrowLen(d.tmp, int(d.header.SystemNameSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].SystemName = uint32(v)
	}

	d.tmp = slices.GrowLen(d.tmp, int(d.header.FileNameSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].Filename = uint32(v)
	}

	d.tmp = slices.GrowLen(d.tmp, int(d.header.StartLineSize))
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.ints, err = enc.DecodeInt32(d.ints, d.tmp)
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].StartLine = uint32(v)
	}

	return nil
}
