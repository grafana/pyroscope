package gosym

import (
	"math"

	"golang.org/x/exp/slices"
)

type PCIndex struct {
	i32 []uint32
	i64 []uint64
}

func NewPCIndex(sz int) PCIndex {
	return PCIndex{
		i32: make([]uint32, sz),
		i64: nil,
	}
}

func (it *PCIndex) Set(idx int, value uint64) {
	if it.i32 != nil && value < math.MaxUint32 {
		it.i32[idx] = uint32(value)
		return
	}
	it.setImpl(idx, value)
}

func (it *PCIndex) setImpl(idx int, value uint64) {
	if it.i32 != nil {
		if value >= math.MaxUint32 {
			Values64 := make([]uint64, len(it.i32))
			for j := 0; j < idx; j++ {
				Values64[j] = uint64(it.i32[j])
			}
			it.i32 = nil
			Values64[idx] = value
			it.i64 = Values64
		} else {
			it.i32[idx] = uint32(value)
		}
	} else {
		it.i64[idx] = value
	}
}

func (it *PCIndex) Length() int {
	if it.i32 != nil {
		return len(it.i32)
	}
	return len(it.i64)
}

func (it *PCIndex) Get(idx int) uint64 {
	if it.i32 != nil {
		return uint64(it.i32[idx])
	}
	return it.i64[idx]
}

func (it *PCIndex) Is32() bool {
	return it.i32 != nil
}
func (it *PCIndex) First() uint64 {
	if it.i32 != nil {
		return uint64(it.i32[0])
	}
	return it.i64[0]
}

func (it *PCIndex) FindIndex(addr uint64) int {
	if it.i32 != nil {

		if addr < uint64(it.i32[0]) {
			return -1
		}
		i, found := slices.BinarySearch(it.i32, uint32(addr))
		if found {
			return i
		}
		i--
		v := it.i32[i]
		for i > 0 && it.i32[i-1] == v {
			i--
		}
		return i
	}
	if addr < it.i64[0] {
		return -1
	}
	i, found := slices.BinarySearch(it.i64, addr)
	if found {
		return i
	}
	i--
	v := it.i64[i]
	for i > 0 && it.i64[i-1] == v {
		i--
	}
	return i
}

func (it *PCIndex) Value(idx int) uint64 {
	if it.i32 != nil {
		return uint64(it.i32[idx])
	}
	return it.i64[idx]
}
func (it *PCIndex) PCIndex64() PCIndex {
	res := *it
	if it.i64 != nil {
		return res
	}
	res.i64 = make([]uint64, len(it.i32))
	for i := 0; i < len(it.i32); i++ {
		res.i64[i] = uint64(res.i32[i])
	}
	res.i32 = nil
	return res
}
