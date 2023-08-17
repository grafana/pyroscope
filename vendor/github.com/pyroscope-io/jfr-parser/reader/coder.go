package reader

import (
	"encoding/binary"
	"io"
	"math"
)

var _ io.ByteReader = (*decoder)(nil)

type coder struct {
	order  binary.ByteOrder
	buf    []byte
	offset int
}

// decoder implements binary.Read() utility but avoid memory escape
type decoder coder

func (d *decoder) bool() bool {
	x := d.buf[d.offset]
	d.offset++
	return x != 0
}

func (d *decoder) ReadByte() (byte, error) {
	if !d.check(1) {
		return 0, io.EOF
	}
	return d.byte(), nil
}

func (d *decoder) check(dataLen int) bool {
	return d.offset+dataLen-1 < len(d.buf)
}

func (d *decoder) uint8() uint8 {
	x := d.buf[d.offset]
	d.offset++
	return x
}

func (d *decoder) uint16() uint16 {
	x := d.order.Uint16(d.buf[d.offset : d.offset+2])
	d.offset += 2
	return x
}

func (d *decoder) uint32() uint32 {
	x := d.order.Uint32(d.buf[d.offset : d.offset+4])
	d.offset += 4
	return x
}

func (d *decoder) uint64() uint64 {
	x := d.order.Uint64(d.buf[d.offset : d.offset+8])
	d.offset += 8
	return x
}
func (d *decoder) float32() float32 {
	x := math.Float32frombits(d.order.Uint32(d.buf[d.offset : d.offset+4]))
	d.offset += 4
	return x
}

func (d *decoder) float64() float64 {
	x := math.Float64frombits(d.order.Uint64(d.buf[d.offset : d.offset+8]))
	d.offset += 8
	return x
}

func (d *decoder) int8() int8 { return int8(d.uint8()) }

func (d *decoder) int16() int16 { return int16(d.uint16()) }

func (d *decoder) int32() int32 { return int32(d.uint32()) }

func (d *decoder) int64() int64 { return int64(d.uint64()) }

func (d *decoder) byte() byte { return byte(d.int8()) }
