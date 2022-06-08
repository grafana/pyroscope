package bits

import (
	"math/bits"
)

type uint128 = [16]byte

func BitCount(count int) uint {
	return 8 * uint(count)
}

func ByteCount(count uint) int {
	return int((count + 7) / 8)
}

func Round(count uint) uint {
	return BitCount(ByteCount(count))
}

func NearestPowerOfTwo(v int) int {
	return int(NearestPowerOfTwo64(uint64(v)))
}

func NearestPowerOfTwo32(v uint32) uint32 {
	return 1 << uint(bits.Len32(v-1))
}

func NearestPowerOfTwo64(v uint64) uint64 {
	return 1 << uint(bits.Len64(v-1))
}

func Len8(i int8) int {
	return bits.Len8(uint8(i))
}

func Len16(i int16) int {
	return bits.Len16(uint16(i))
}

func Len32(i int32) int {
	return bits.Len32(uint32(i))
}

func Len64(i int64) int {
	return bits.Len64(uint64(i))
}

func IndexShift8(bitIndex uint) (index, shift uint) {
	return bitIndex / 8, bitIndex % 8
}

func IndexShift16(bitIndex uint) (index, shift uint) {
	return bitIndex / 16, bitIndex % 16
}

func IndexShift32(bitIndex uint) (index, shift uint) {
	return bitIndex / 32, bitIndex % 32
}

func IndexShift64(bitIndex uint) (index, shift uint) {
	return bitIndex / 64, bitIndex % 64
}
