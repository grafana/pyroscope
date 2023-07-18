package cappedarr

import (
	"sort"
)

type CappedArray struct {
	len     int
	maxSize int
	values  []uint64
}

func New(maxSize int) *CappedArray {
	return &CappedArray{
		len:     0,
		maxSize: maxSize,
		values:  make([]uint64, maxSize),
	}
}

func (ca *CappedArray) MinValue() uint64 {
	if ca.len == 0 {
		return 0
	}
	return ca.values[ca.maxSize-ca.len]
}

func (ca *CappedArray) Push(v uint64) bool {
	i := sort.Search(ca.len, func(i int) bool {
		return ca.values[ca.maxSize-ca.len+i] >= v
	})

	// log.Debug("---")

	if i < 0 {
		return false
	}

	if ca.len < ca.maxSize {
		ca.len++
	}

	// log.Debugf("maxSize %d len %d i %d v %d first val %d", ca.maxSize, ca.len, i, v, ca.values[0])

	if i >= ca.maxSize {
		// log.Debug("case 1")
		copy(ca.values[:i-1], ca.values[1:i])
		ca.values[i-1] = v
	} else {
		if i == 0 && v <= ca.values[0] {
			// log.Debug("case 2")
			return false
		}
		if v > ca.values[ca.maxSize-ca.len+i] {
			// log.Debug("case 3")
			if i != 0 {
				l := ca.maxSize - ca.len
				r := l + i
				copy(ca.values[l:r], ca.values[l+1:r+1])
			}
			ca.values[ca.maxSize-ca.len+i] = v
		} else {
			// log.Debug("case 4")
			copy(ca.values[:i-1], ca.values[1:i])
			ca.values[ca.maxSize-ca.len+i-1] = v
		}
	}
	// log.Debug("ca.values", ca.values)

	// log.Debug(i)

	// if i >= ca.len && ca.len < len(ca.values)-1 {
	// 	ca.len += 1
	// 	copy(ca.values[i+1:ca.len], ca.values[i:ca.len-1])
	// } else {
	// 	copy(ca.values[:i-1], ca.values[1:i])
	// }

	// ca.values[i] = v
	return true
}
