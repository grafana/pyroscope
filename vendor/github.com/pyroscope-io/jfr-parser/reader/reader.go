package reader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unsafe"
)

type VarReader interface {
	VarShort() (int16, error)
	VarInt() (int32, error)
	VarLong() (int64, error)
}

type Reader interface {
	Boolean() (bool, error)
	Byte() (int8, error)
	Short() (int16, error)
	Char() (uint16, error)
	Int() (int32, error)
	Long() (int64, error)
	Float() (float32, error)
	Double() (float64, error)
	String() (string, error)
	SeekStart(offset int64) (int64, error)
	Offset() int
	VarReader

	// TODO: Support arrays
}

type reader struct {
	varR VarReader
	*decoder
	unsafeByteToString bool
}

func NewReader(b []byte, compressed bool, unsafeByteToString bool) Reader {
	d := &decoder{
		order:  binary.BigEndian,
		buf:    b,
		offset: 0,
	}
	var varR VarReader
	if compressed {
		varR = newCompressed(d)
	} else {
		varR = newUncompressed(d)
	}
	return reader{
		varR:               varR,
		decoder:            d,
		unsafeByteToString: unsafeByteToString,
	}
}

func (r reader) Offset() int {
	return r.offset
}
func (r reader) Boolean() (bool, error) {
	if !r.check(1) {
		return false, io.EOF
	}
	return r.bool(), nil
}

// SeekStart implement Seek(offset, io.SeekStart)
func (r reader) SeekStart(offset int64) (int64, error) {
	abs := offset
	r.offset = int(abs)
	if abs < 0 {
		return 0, errors.New("bytes.Reader.Seek: negative position")
	}
	return abs, nil
}

func (r reader) Byte() (int8, error) {
	if !r.check(1) {
		return 0, io.EOF
	}
	return r.int8(), nil
}

func (r reader) Short() (int16, error) {
	if !r.check(2) {
		return 0, io.EOF
	}
	return r.int16(), nil
}

func (r reader) Char() (uint16, error) {
	if !r.check(2) {
		return 0, io.EOF
	}
	return r.uint16(), nil
}

func (r reader) Int() (int32, error) {
	if !r.check(4) {
		return 0, io.EOF
	}
	return r.int32(), nil
}

func (r reader) Long() (int64, error) {
	if !r.check(8) {
		return 0, io.EOF
	}
	return r.int64(), nil
}

func (r reader) Float() (float32, error) {
	if !r.check(4) {
		return 0, io.EOF
	}
	return r.float32(), nil
}

func (r reader) Double() (float64, error) {
	if !r.check(8) {
		return 0, io.EOF
	}
	return r.float64(), nil
}

// TODO: Should we differentiate between null and empty?
func (r reader) String() (string, error) {
	enc, err := r.Byte()
	if err != nil {
		return "", err
	}
	switch enc {
	case 0:
		return "", nil
	case 1:
		return "", nil
	case 3, 4, 5:
		return r.utf8()
	default:
		// TODO
		return "", fmt.Errorf("Unsupported string type :%d", enc)
	}
}

func (r reader) VarShort() (int16, error) {
	return r.varR.VarShort()
}

func (r reader) VarInt() (int32, error) {
	return r.varR.VarInt()
}

func (r reader) VarLong() (int64, error) {
	return r.varR.VarLong()
}

func (r reader) utf8() (string, error) {
	n, err := r.varR.VarInt()
	if err != nil {
		return "", err
	}
	if !r.check(int(n)) {
		return "", io.EOF
	}
	b := r.decoder.buf[r.decoder.offset : r.decoder.offset+int(n)]
	r.decoder.offset += int(n)
	if r.unsafeByteToString {
		return BytesToString(b), err
	}
	return string(b), nil
}

// BytesToString converts byte slice to string without a memory allocation.
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
