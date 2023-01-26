package streaming

import (
	"github.com/cespare/xxhash/v2"
	"reflect"
	"unsafe"
)

func (p *VTStreamingParser) parseSampleWB(buffer []byte) error {
	p.tmpSample.reset(p.arena)
	err := p.tmpSample.UnmarshalSampleVT(buffer, p.arena)
	if err != nil {
		return err
	}

	for i := len(p.tmpSample.tmpStackLoc) - 1; i >= 0; i-- {
		err = p.addStackLocation(p.tmpSample.tmpStackLoc[i])
		if err != nil {
			return err
		}
	}
	h := uint64(0)
	if p.cumulative {
		h = hashStack(p.tmpSample.tmpStack)
	}

	for _, vi := range p.indexes {
		v := uint64(p.tmpSample.tmpValues[vi])
		if v == 0 {
			continue
		}
		wb := p.wbCache.getWriteBatch(p, vi)
		if wb == nil {
			continue
		}

		if p.cumulative {
			if p.prev {
				wb.prev[h] += v
				continue
			} else {
				prevVal, ok := wb.prev[h]
				if ok {
					if prevVal > v {
						prevVal -= v
						wb.prev[h] = prevVal
						continue
					}
					v -= prevVal
					wb.prev[h] = 0
					if v < 0 {
						print("cumulative diff negative value")
					}
					if v == 0 {
						continue
					}
				}
			}
		}

		sb := wb.wb.StackBuilder()
		sb.Reset()
		for _, frame := range p.tmpSample.tmpStack {
			sb.Push(frame)
		}
		stackID := sb.Build()

		wb.getAppender(p, p.tmpSample.tmpLabels).Append(stackID, v)

		if !p.cumulative {
			if j := findLabelIndex(p.tmpSample.tmpLabels, p.profileIDLabelIndex); j >= 0 {
				copy(p.tmpSample.tmpLabels[j:], p.tmpSample.tmpLabels[j+1:])
				p.tmpSample.tmpLabels = p.tmpSample.tmpLabels[:len(p.tmpSample.tmpLabels)-1]
				wb.getAppender(p, p.tmpSample.tmpLabels).Append(stackID, v)
			}
		}
	}
	return nil
}

func hashStack(stack [][]byte) uint64 {
	if len(stack) == 0 {
		return zeroHash
	}
	var hashes [64 + 32]uint64
	s := hashes[:]

	for _, frame := range stack {
		h := xxhash.Sum64(frame)
		s = append(s, h)
	}

	var raw []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&raw))
	sh.Data = uintptr(unsafe.Pointer(&s[0]))
	sh.Len = len(s) * 8
	sh.Cap = len(s) * 8
	res := xxhash.Sum64(raw)
	return res
}
