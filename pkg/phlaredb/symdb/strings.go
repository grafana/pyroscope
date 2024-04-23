package symdb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"

	"github.com/grafana/pyroscope/pkg/slices"
)

const (
	maxStringLen           = 1<<16 - 1
	stringsBlockHeaderSize = int(unsafe.Sizeof(stringsBlockHeader{}))
)

var (
	_ symbolsBlockEncoder[string] = (*stringsBlockEncoder)(nil)
	_ symbolsBlockDecoder[string] = (*stringsBlockDecoder)(nil)
)

type stringsBlockHeader struct {
	StringsLen    uint32
	BlockEncoding byte
	_             [3]byte
}

func (h *stringsBlockHeader) marshal(b []byte) {
	binary.BigEndian.PutUint32(b[0:4], h.StringsLen)
	b[5] = h.BlockEncoding
}

func (h *stringsBlockHeader) unmarshal(b []byte) {
	h.StringsLen = binary.BigEndian.Uint32(b[0:4])
	h.BlockEncoding = b[5]
}

type stringsBlockEncoder struct {
	header stringsBlockHeader
	buf    bytes.Buffer
	tmp    []byte
}

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
	e.tmp = slices.GrowLen(e.tmp, stringsBlockHeaderSize)
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
	header stringsBlockHeader
	tmp    []byte
}

func (d *stringsBlockDecoder) readHeader(r io.Reader) error {
	d.tmp = slices.GrowLen(d.tmp, stringsBlockHeaderSize)
	if _, err := io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	d.header.unmarshal(d.tmp)
	if d.header.BlockEncoding == 8 || d.header.BlockEncoding == 16 {
		return nil
	}
	return fmt.Errorf("invalid string block encoding: %d", d.header.BlockEncoding)
}

func (d *stringsBlockDecoder) decode(r io.Reader, strings []string) (err error) {
	if err = d.readHeader(r); err != nil {
		return err
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
	d.tmp = slices.GrowLen(d.tmp, len(dst)) // 1 byte per string.
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	for i := 0; i < len(dst); i++ {
		s := make([]byte, d.tmp[i])
		if _, err = io.ReadFull(r, s); err != nil {
			return err
		}
		dst[i] = *(*string)(unsafe.Pointer(&s))
	}
	return err
}

func (d *stringsBlockDecoder) decodeStrings16(r io.Reader, dst []string) (err error) {
	d.tmp = slices.GrowLen(d.tmp, len(dst)*2) // 2 bytes per string.
	if _, err = io.ReadFull(r, d.tmp); err != nil {
		return err
	}
	for i := 0; i < len(dst); i++ {
		l := binary.BigEndian.Uint16(d.tmp[i*2:])
		s := make([]byte, l)
		if _, err = io.ReadFull(r, s); err != nil {
			return err
		}
		dst[i] = *(*string)(unsafe.Pointer(&s))
	}
	return err
}
