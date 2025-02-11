//nolint:unused
package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"unsafe"

	"github.com/grafana/pyroscope/pkg/slices"
)

const maxStringLen = 1<<16 - 1

var (
	_ symbolsBlockEncoder[string] = (*stringsBlockEncoder)(nil)
	_ symbolsBlockDecoder[string] = (*stringsBlockDecoder)(nil)
)

type stringsBlockHeader struct {
	StringsLen    uint32
	BlockEncoding byte
	_             [3]byte
	CRC           uint32
}

func (h *stringsBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint32(b[0:4], h.StringsLen)
	b[5], b[6], b[7], b[8] = h.BlockEncoding, 0, 0, 0
	// Fields can be added here in the future.
	// CRC must be the last four bytes.
	h.CRC = crc32.Checksum(b[0:8], castagnoli)
	binary.BigEndian.PutUint32(b[8:12], h.CRC)
}

func (h *stringsBlockHeader) unmarshal(b []byte) {
	h.StringsLen = binary.BigEndian.Uint32(b[0:4])
	h.BlockEncoding = b[5]
	// In future versions, new fields are decoded here;
	// if pos < len(b)-checksumSize, then there are more fields.
	h.CRC = binary.BigEndian.Uint32(b[8:12])
}

func (h *stringsBlockHeader) checksum() uint32 { return h.CRC }

type stringsBlockEncoder struct {
	header stringsBlockHeader
	buf    bytes.Buffer
	tmp    []byte
}

func newStringsEncoder() *symbolsEncoder[string] {
	return newSymbolsEncoder[string](new(stringsBlockEncoder))
}

func (e *stringsBlockEncoder) format() SymbolsBlockFormat { return BlockStringsV1 }

func (e *stringsBlockEncoder) headerSize() uintptr { return unsafe.Sizeof(stringsBlockHeader{}) }

func (e *stringsBlockEncoder) encode(w io.Writer, strings []string) error {
	e.initWrite(len(strings))
	e.header.BlockEncoding = e.blockEncoding(strings)
	switch e.header.BlockEncoding {
	case 8:
		for j, s := range strings {
			e.tmp[j] = byte(len(s))
		}
	case 16:
		for j, s := range strings {
			binary.BigEndian.PutUint16(e.tmp[j*2:], uint16(len(s)))
		}
	}
	if _, err := e.buf.Write(e.tmp[:len(strings)*int(e.header.BlockEncoding)/8]); err != nil {
		return err
	}
	for _, s := range strings {
		if len(s) > maxStringLen {
			s = s[:maxStringLen]
		}
		if _, err := e.buf.Write(*((*[]byte)(unsafe.Pointer(&s)))); err != nil {
			return err
		}
	}
	e.tmp = slices.GrowLen(e.tmp, int(e.headerSize()))
	e.header.marshal(e.tmp)
	if _, err := w.Write(e.tmp); err != nil {
		return err
	}
	_, err := e.buf.WriteTo(w)
	return err
}

func (e *stringsBlockEncoder) blockEncoding(b []string) byte {
	for _, s := range b {
		if len(s) > 255 {
			return 16
		}
	}
	return 8
}

func (e *stringsBlockEncoder) initWrite(strings int) {
	e.buf.Reset()
	e.buf.Grow(strings * 16)
	*e = stringsBlockEncoder{
		header: stringsBlockHeader{StringsLen: uint32(strings)},
		tmp:    slices.GrowLen(e.tmp, strings*2),
		buf:    e.buf,
	}
}

type stringsBlockDecoder struct {
	headerSize uint16
	header     stringsBlockHeader
	buf        []byte
}

func newStringsDecoder(h SymbolsBlockHeader) (*symbolsDecoder[string], error) {
	if h.Format == BlockStringsV1 {
		headerSize := max(stringsBlockHeaderMinSize, h.BlockHeaderSize)
		return newSymbolsDecoder[string](h, &stringsBlockDecoder{headerSize: headerSize}), nil
	}
	return nil, fmt.Errorf("%w: unknown strings format: %d", ErrUnknownVersion, h.Format)
}

// In early versions, block header size is not specified. Must not change.
const stringsBlockHeaderMinSize = 12

func (d *stringsBlockDecoder) decode(r io.Reader, strings []string) (err error) {
	d.buf = slices.GrowLen(d.buf, int(d.headerSize))
	if err = readSymbolsBlockHeader(d.buf, r, &d.header); err != nil {
		return err
	}
	if d.header.BlockEncoding != 8 && d.header.BlockEncoding != 16 {
		return fmt.Errorf("invalid string block encoding: %d", d.header.BlockEncoding)
	}
	if d.header.StringsLen != uint32(len(strings)) {
		return fmt.Errorf("invalid string buffer size")
	}
	if d.header.BlockEncoding == 8 {
		return d.decodeStrings8(r, strings)
	}
	return d.decodeStrings16(r, strings)
}

func (d *stringsBlockDecoder) decodeStrings8(r io.Reader, dst []string) (err error) {
	d.buf = slices.GrowLen(d.buf, len(dst)) // 1 byte per string.
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	for i := 0; i < len(dst); i++ {
		s := make([]byte, d.buf[i])
		if _, err = io.ReadFull(r, s); err != nil {
			return err
		}
		dst[i] = *(*string)(unsafe.Pointer(&s))
	}
	return err
}

func (d *stringsBlockDecoder) decodeStrings16(r io.Reader, dst []string) (err error) {
	d.buf = slices.GrowLen(d.buf, len(dst)*2) // 2 bytes per string.
	if _, err = io.ReadFull(r, d.buf); err != nil {
		return err
	}
	for i := 0; i < len(dst); i++ {
		l := binary.BigEndian.Uint16(d.buf[i*2:])
		s := make([]byte, l)
		if _, err = io.ReadFull(r, s); err != nil {
			return err
		}
		dst[i] = *(*string)(unsafe.Pointer(&s))
	}
	return err
}
