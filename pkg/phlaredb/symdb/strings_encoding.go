package symdb

import (
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"

	"github.com/grafana/pyroscope/pkg/slices"
)

// Almost all strings in profiles are very short, their length fits 8 bits.
// Strings larger than 65536 are not expected and are getting truncated.
// Typically, there are only 1-10 strings longer than 256 in a data set
// consisting of a few dozens of thousands of strings.
//
// A traditional var length encoding is rather wasteful in our case.
// Instead, we split the strings into blocks and use encoding that depends
// on the maximum length of the strings in the block.
//
// The output data starts with a header: number of strings, block size,
// number of blocks, and the block encoding map. In the map, each byte
// specifies the number of bits needed to decode the maximum value from
// that block, rounded up to the next power of two. Currently, the length
// value is either 8 bits or 16.
//
// Blocks of data follow after the header. Each block includes two parts:
// strings lengths array and strings data.

const maxStringLen = 1<<16 - 1

type StringsEncoder struct {
	w         io.Writer
	blockSize int
	blocks    []byte
	buf       []byte
}

func NewStringsEncoder(w io.Writer) *StringsEncoder { return &StringsEncoder{w: w} }

func (e *StringsEncoder) WriteStrings(strings []string) error {
	if e.blockSize == 0 {
		e.blockSize = 1 << 10 // 1k strings per block by default.
	}
	nb := (len(strings) + e.blockSize - 1) / e.blockSize
	e.blocks = slices.GrowLen(e.blocks, nb)
	var offset uint32
	var bi int
	l := uint32(len(strings))
	for offset < l {
		lo := offset
		hi := offset + uint32(e.blockSize)
		if x := uint32(len(strings)); hi > x {
			hi = x
		}
		e.blocks[bi] = e.blockEncoding(strings[lo:hi])
		offset = hi
		bi++
	}
	if err := e.writeHeader(strings); err != nil {
		return err
	}
	// Next we write string lengths and values in blocks.
	e.buf = slices.GrowLen(e.buf, e.blockSize*2) // Up to 2 bytes per string.
	for i, b := range e.blocks {
		// e.buf = e.buf[:0]
		lo := i * e.blockSize
		hi := lo + e.blockSize
		if x := len(strings); hi > x {
			hi = x
		}
		bs := strings[lo:hi]
		switch b {
		case 8:
			for j, s := range bs {
				e.buf[j] = byte(len(s))
			}
		case 16:
			for j, s := range bs {
				// binary.LittleEndian.PutUint16.
				e.buf[j*2] = byte(len(s))
				e.buf[j*2+1] = byte(len(s) >> 8)
			}
		default:
			panic("bug: unexpected block size")
		}
		if _, err := e.w.Write(e.buf[:len(bs)*int(b)/8]); err != nil {
			return err
		}
		for _, s := range bs {
			if len(s) > maxStringLen {
				s = s[:maxStringLen]
			}
			if _, err := e.w.Write(*((*[]byte)(unsafe.Pointer(&s)))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *StringsEncoder) writeHeader(strings []string) (err error) {
	e.buf = slices.GrowLen(e.buf, 12)
	binary.LittleEndian.PutUint32(e.buf[0:4], uint32(len(strings)))
	binary.LittleEndian.PutUint32(e.buf[4:8], uint32(e.blockSize))
	binary.LittleEndian.PutUint32(e.buf[8:12], uint32(len(e.blocks)))
	if _, err = e.w.Write(e.buf); err != nil {
		return err
	}
	_, err = e.w.Write(e.blocks)
	return err
}

func (e *StringsEncoder) blockEncoding(b []string) byte {
	var x uint16
	for _, s := range b {
		x |= uint16(len(s)) >> 8
	}
	if x > 0 {
		return 16
	}
	return 8
}

func (e *StringsEncoder) Reset() {
	e.buf = e.buf[:0]
	e.blocks = e.blocks[:0]
	e.blockSize = 0
	e.w = nil
}

type StringsDecoder struct {
	r          io.Reader
	stringsLen uint32
	blocksLen  uint32
	blockSize  uint32
	blocks     []byte
	buf        []byte
}

func NewStringsDecoder(r io.Reader) *StringsDecoder { return &StringsDecoder{r: r} }

func (d *StringsDecoder) readHeader() (err error) {
	d.buf = slices.GrowLen(d.buf, 12)
	if _, err = io.ReadFull(d.r, d.buf); err != nil {
		return err
	}
	d.stringsLen = binary.LittleEndian.Uint32(d.buf[0:4])
	d.blockSize = binary.LittleEndian.Uint32(d.buf[4:8])
	d.blocksLen = binary.LittleEndian.Uint32(d.buf[8:12])
	// Sanity checks are needed as we process the stream data
	// before verifying the check sum.
	if d.blocksLen > 1<<20 || d.stringsLen > 1<<20 || d.blockSize > 1<<20 {
		return fmt.Errorf("malformed header")
	}
	d.blocks = slices.GrowLen(d.blocks, int(d.blocksLen))
	_, err = io.ReadFull(d.r, d.blocks)
	return err
}

func (d *StringsDecoder) StringsLen() (int, error) {
	if err := d.readHeader(); err != nil {
		return 0, err
	}
	return int(d.stringsLen), nil
}

func (d *StringsDecoder) ReadStrings(dst []string) (err error) {
	for i := 0; i < len(d.blocks); i++ {
		bs := d.blockSize
		if i == len(d.blocks)-1 && d.stringsLen%d.blockSize > 0 {
			bs = d.stringsLen % d.blockSize
		}
		switch d.blocks[i] {
		case 8:
			err = d.readStrings8(i, int(bs), dst)
		case 16:
			err = d.readStrings16(i, int(bs), dst)
		default:
			err = fmt.Errorf("unknown block encoding")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *StringsDecoder) readStrings8(idx, length int, dst []string) (err error) {
	d.buf = slices.GrowLen(d.buf, length) // 1 byte per string.
	if _, err = io.ReadFull(d.r, d.buf); err != nil {
		return err
	}
	offset := int(d.blockSize) * idx
	for i, l := range d.buf {
		s := make([]byte, l) // Up to 256 bytes.
		if _, err = io.ReadFull(d.r, s); err != nil {
			return err
		}
		dst[offset+i] = *(*string)(unsafe.Pointer(&s))
	}
	return err
}

func (d *StringsDecoder) readStrings16(idx, length int, dst []string) (err error) {
	d.buf = slices.GrowLen(d.buf, length*2) // 2 bytes per string.
	if _, err = io.ReadFull(d.r, d.buf); err != nil {
		return err
	}
	offset := int(d.blockSize) * idx
	for i := 0; i < len(d.buf); i += 2 {
		l := uint16(d.buf[i]) | uint16(d.buf[i+1])<<8
		s := make([]byte, l) // Up to 65536 bytes.
		if _, err = io.ReadFull(d.r, s); err != nil {
			return err
		}
		dst[offset+i/2] = *(*string)(unsafe.Pointer(&s))
	}
	return err
}

func (d *StringsDecoder) Reset() {
	d.buf = d.buf[:0]
	d.blocks = d.blocks[:0]
	d.blockSize = 0
	d.blocksLen = 0
	d.stringsLen = 0
	d.r = nil
}
