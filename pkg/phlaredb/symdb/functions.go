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
	CRC            uint32
}

func (h *functionsBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint32(b[0:4], h.FunctionsLen)
	binary.BigEndian.PutUint32(b[4:8], h.NameSize)
	binary.BigEndian.PutUint32(b[8:12], h.SystemNameSize)
	binary.BigEndian.PutUint32(b[12:16], h.FileNameSize)
	binary.BigEndian.PutUint32(b[16:20], h.StartLineSize)
	h.CRC = crc32.Checksum(b[0:20], castagnoli)
	binary.BigEndian.PutUint32(b[20:24], h.CRC)
}

func (h *functionsBlockHeader) unmarshal(b []byte) {
	h.FunctionsLen = binary.BigEndian.Uint32(b[0:4])
	h.NameSize = binary.BigEndian.Uint32(b[4:8])
	h.SystemNameSize = binary.BigEndian.Uint32(b[8:12])
	h.FileNameSize = binary.BigEndian.Uint32(b[12:16])
	h.StartLineSize = binary.BigEndian.Uint32(b[16:20])
	h.CRC = binary.BigEndian.Uint32(b[20:24])
}

type functionsBlockEncoder struct {
	header functionsBlockHeader

	tmp  []byte
	buf  bytes.Buffer
	ints []int32
}

func newFunctionsEncoder() *symbolsEncoder[v1.InMemoryFunction] {
	return newSymbolsEncoder[v1.InMemoryFunction](new(functionsBlockEncoder))
}

func (e *functionsBlockEncoder) format() SymbolsBlockFormat {
	return BlockFunctionsV1
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
	format SymbolsBlockFormat
	header functionsBlockHeader

	ints []int32
	buf  []byte
}

func newFunctionsDecoder(h SymbolsBlockHeader) (*symbolsDecoder[v1.InMemoryFunction], error) {
	if h.Format == BlockFunctionsV1 {
		return newSymbolsDecoder[v1.InMemoryFunction](h, &functionsBlockDecoder{format: h.Format}), nil
	}
	return nil, fmt.Errorf("%w: unknown functions format: %d", ErrUnknownVersion, h.Format)
}

func (d *functionsBlockDecoder) readHeader(r io.Reader) error {
	d.buf = slices.GrowLen(d.buf, functionsBlockHeaderSize)
	if _, err := io.ReadFull(r, d.buf); err != nil {
		return nil
	}
	d.header.unmarshal(d.buf)
	if crc32.Checksum(d.buf[:functionsBlockHeaderSize-4], castagnoli) != d.header.CRC {
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

	d.ints = slices.GrowLen(d.ints, int(d.header.FunctionsLen))
	d.buf = slices.GrowLen(d.buf, int(d.header.NameSize))
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	d.ints, err = decodeBinaryPackedInt32(d.ints, d.buf, int(d.header.FunctionsLen))
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].Name = uint32(v)
	}

	d.buf = slices.GrowLen(d.buf, int(d.header.SystemNameSize))
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	d.ints, err = decodeBinaryPackedInt32(d.ints, d.buf, int(d.header.FunctionsLen))
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].SystemName = uint32(v)
	}

	d.buf = slices.GrowLen(d.buf, int(d.header.FileNameSize))
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	d.ints, err = decodeBinaryPackedInt32(d.ints, d.buf, int(d.header.FunctionsLen))
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].Filename = uint32(v)
	}

	d.buf = slices.GrowLen(d.buf, int(d.header.StartLineSize))
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	d.ints, err = decodeBinaryPackedInt32(d.ints, d.buf, int(d.header.FunctionsLen))
	if err != nil {
		return err
	}
	for i, v := range d.ints {
		functions[i].StartLine = uint32(v)
	}

	return nil
}
