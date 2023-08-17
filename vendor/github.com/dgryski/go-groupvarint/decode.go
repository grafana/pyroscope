// +build !amd64 noasm

package groupvarint

import "encoding/binary"

var mask = [4]uint32{0xff, 0xffff, 0xffffff, 0xffffffff}

func Decode4(dst []uint32, src []byte) {
	bits := src[0]
	src = src[1:]

	b := bits & 3
	n := load(src, mask[b])
	src = src[1+b:]
	bits >>= 2
	dst[0] = uint32(n)

	b = bits & 3
	n = load(src, mask[b])
	src = src[1+b:]
	bits >>= 2
	dst[1] = uint32(n)

	b = bits & 3
	n = load(src, mask[b])
	src = src[1+b:]
	bits >>= 2
	dst[2] = uint32(n)

	b = bits & 3
	n = load(src, mask[b])
	dst[3] = uint32(n)
}

func load(src []byte, mask uint32) uint32 {
	if len(src) > 4 {
		return binary.LittleEndian.Uint32(src) & mask
	}

	switch mask {
	case 0xff:
		return uint32(src[0])
	case 0xffff:
		return uint32(binary.LittleEndian.Uint16(src))
	case 0xffffff:
		return uint32(binary.LittleEndian.Uint16(src)) | uint32(src[2])<<16
	case 0xffffffff:
		return binary.LittleEndian.Uint32(src)
	}

	return 0
}
