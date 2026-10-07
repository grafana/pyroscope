package varint

import (
	"encoding/binary"
	"io"
)

type Writer []byte

func NewWriter() Writer {
	return make([]byte, binary.MaxVarintLen64)
}

func (buf Writer) Write(w io.Writer, val uint64) (int, error) {
	n := binary.PutUvarint(buf, val)
	return w.Write(buf[:n])
}

func Write(w io.Writer, val uint64) (int, error) {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, val)
	return w.Write(buf[:n])
}

func Read(r io.ByteReader) (uint64, error) {
	return binary.ReadUvarint(r)
}

// Uvarint decodes a uint64 from buf and returns that value and the
// number of bytes read (> 0). If an error occurred, the value is 0
// and the number of bytes n is <= 0 meaning:
//
//	n == 0: buf too small
//	n  < 0: value larger than 64 bits (overflow)
//	        and -n is the number of bytes read
//
// Copied from https://github.com/dennwc/varint
func Uvarint(buf []byte) (uint64, int) {
	// Fully unrolled implementation of binary.Uvarint.
	//
	// It will also eliminate bound checks for buffers larger than 9 bytes.
	sz := len(buf)
	if sz == 0 {
		return 0, 0
	}
	const (
		step = 7
		bit  = 1 << 7
		mask = bit - 1
	)
	if sz >= 10 { // no bound checks
		// i == 0
		b := buf[0]
		if b < bit {
			return uint64(b), 1
		}
		x := uint64(b & mask)
		var s uint = step

		// i == 1
		b = buf[1]
		if b < bit {
			return x | uint64(b)<<s, 2
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 2
		b = buf[2]
		if b < bit {
			return x | uint64(b)<<s, 3
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 3
		b = buf[3]
		if b < bit {
			return x | uint64(b)<<s, 4
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 4
		b = buf[4]
		if b < bit {
			return x | uint64(b)<<s, 5
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 5
		b = buf[5]
		if b < bit {
			return x | uint64(b)<<s, 6
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 6
		b = buf[6]
		if b < bit {
			return x | uint64(b)<<s, 7
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 7
		b = buf[7]
		if b < bit {
			return x | uint64(b)<<s, 8
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 8
		b = buf[8]
		if b < bit {
			return x | uint64(b)<<s, 9
		}
		x |= uint64(b&mask) << s
		s += step

		// i == 9
		b = buf[9]
		if b < bit {
			if b > 1 {
				return 0, -10 // overflow
			}
			return x | uint64(b)<<s, 10
		} else if sz == 10 {
			return 0, 0
		}
		for j, b := range buf[10:] {
			if b < bit {
				return 0, -(11 + j)
			}
		}
		return 0, 0
	}

	// i == 0
	b := buf[0]
	if b < bit {
		return uint64(b), 1
	} else if sz == 1 {
		return 0, 0
	}
	x := uint64(b & mask)
	var s uint = step

	// i == 1
	b = buf[1]
	if b < bit {
		return x | uint64(b)<<s, 2
	} else if sz == 2 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 2
	b = buf[2]
	if b < bit {
		return x | uint64(b)<<s, 3
	} else if sz == 3 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 3
	b = buf[3]
	if b < bit {
		return x | uint64(b)<<s, 4
	} else if sz == 4 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 4
	b = buf[4]
	if b < bit {
		return x | uint64(b)<<s, 5
	} else if sz == 5 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 5
	b = buf[5]
	if b < bit {
		return x | uint64(b)<<s, 6
	} else if sz == 6 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 6
	b = buf[6]
	if b < bit {
		return x | uint64(b)<<s, 7
	} else if sz == 7 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 7
	b = buf[7]
	if b < bit {
		return x | uint64(b)<<s, 8
	} else if sz == 8 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 8
	b = buf[8]
	if b < bit {
		return x | uint64(b)<<s, 9
	} else if sz == 9 {
		return 0, 0
	}
	x |= uint64(b&mask) << s
	s += step

	// i == 9
	b = buf[9]
	if b < bit {
		if b > 1 {
			return 0, -10 // overflow
		}
		return x | uint64(b)<<s, 10
	} else if sz == 10 {
		return 0, 0
	}
	for j, b := range buf[10:] {
		if b < bit {
			return 0, -(11 + j)
		}
	}
	return 0, 0
}
