//go:build !go1.18

package bits

import "unsafe"

func BoolToBytes(data []bool) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), len(data))
}

func Int8ToBytes(data []int8) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), len(data))
}

func Int16ToBytes(data []int16) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 2*len(data))
}

func Int32ToBytes(data []int32) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 4*len(data))
}

func Int64ToBytes(data []int64) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 8*len(data))
}

func Float32ToBytes(data []float32) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 4*len(data))
}

func Float64ToBytes(data []float64) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 8*len(data))
}

func Int16ToUint16(data []int16) []uint16 {
	return unsafe.Slice(*(**uint16)(unsafe.Pointer(&data)), len(data))
}

func Int32ToUint32(data []int32) []uint32 {
	return unsafe.Slice(*(**uint32)(unsafe.Pointer(&data)), len(data))
}

func Int64ToUint64(data []int64) []uint64 {
	return unsafe.Slice(*(**uint64)(unsafe.Pointer(&data)), len(data))
}

func Float32ToUint32(data []float32) []uint32 {
	return unsafe.Slice(*(**uint32)(unsafe.Pointer(&data)), len(data))
}

func Float64ToUint64(data []float64) []uint64 {
	return unsafe.Slice(*(**uint64)(unsafe.Pointer(&data)), len(data))
}

func Uint32ToBytes(data []uint32) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 4*len(data))
}

func Uint64ToBytes(data []uint64) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 8*len(data))
}

func Uint128ToBytes(data [][16]byte) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&data)), 16*len(data))
}

func Uint32ToInt32(data []uint32) []int32 {
	return unsafe.Slice(*(**int32)(unsafe.Pointer(&data)), len(data))
}

func Uint64ToInt64(data []uint64) []int64 {
	return unsafe.Slice(*(**int64)(unsafe.Pointer(&data)), len(data))
}

func BytesToBool(data []byte) []bool {
	return unsafe.Slice(*(**bool)(unsafe.Pointer(&data)), len(data))
}

func BytesToInt8(data []byte) []int8 {
	return unsafe.Slice(*(**int8)(unsafe.Pointer(&data)), len(data))
}

func BytesToInt16(data []byte) []int16 {
	return unsafe.Slice(*(**int16)(unsafe.Pointer(&data)), len(data)/2)
}

func BytesToInt32(data []byte) []int32 {
	return unsafe.Slice(*(**int32)(unsafe.Pointer(&data)), len(data)/4)
}

func BytesToInt64(data []byte) []int64 {
	return unsafe.Slice(*(**int64)(unsafe.Pointer(&data)), len(data)/8)
}

func BytesToUint32(data []byte) []uint32 {
	return unsafe.Slice(*(**uint32)(unsafe.Pointer(&data)), len(data)/4)
}

func BytesToUint64(data []byte) []uint64 {
	return unsafe.Slice(*(**uint64)(unsafe.Pointer(&data)), len(data)/8)
}

func BytesToUint128(data []byte) [][16]byte {
	return unsafe.Slice(*(**[16]byte)(unsafe.Pointer(&data)), len(data)/16)
}

func BytesToFloat32(data []byte) []float32 {
	return unsafe.Slice(*(**float32)(unsafe.Pointer(&data)), len(data)/4)
}

func BytesToFloat64(data []byte) []float64 {
	return unsafe.Slice(*(**float64)(unsafe.Pointer(&data)), len(data)/8)
}

func BytesToString(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
