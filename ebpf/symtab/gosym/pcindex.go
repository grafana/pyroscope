package gosym

import (
	"math"
	"sort"
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
	n := len(it.i32) + len(it.i64)
	if it.i32 != nil {
		var i int
		if addr < uint64(it.i32[0]) {
			return -1
		}
		i = sort.Search(n, func(i int) bool {
			return addr < uint64(it.i32[i])
		})
		i--
		return i
	}
	var i int
	if addr < it.i64[0] {
		return -1
	}
	i = sort.Search(n, func(i int) bool {
		return addr < it.i64[i]
	})
	i--
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
