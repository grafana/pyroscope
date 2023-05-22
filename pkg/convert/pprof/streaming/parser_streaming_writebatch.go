package streaming

import (
	"context"
	"fmt"
	"github.com/cespare/xxhash/v2"
	"github.com/pyroscope-io/pyroscope/pkg/stackbuilder"
	"github.com/pyroscope-io/pyroscope/pkg/util/arenahelper"
	"reflect"
	"runtime/debug"
	"time"
	"unsafe"
)

type ParseWriteBatchInput struct {
	Context            context.Context
	StartTime, EndTime time.Time
	Profile, Previous  []byte
	WriteBatchFactory  stackbuilder.WriteBatchFactory
}

func (p *VTStreamingParser) ParseWithWriteBatch(input ParseWriteBatchInput) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf(fmt.Sprintf("parse panic %s", debug.Stack()))
		}
	}()
	p.startTime = input.StartTime
	p.endTime = input.EndTime
	p.ctx = input.Context
	p.wbf = input.WriteBatchFactory
	p.cumulative = input.Previous != nil
	p.cumulativeOnly = input.Previous != nil
	p.wbCache.cumulative = p.cumulative
	if input.Previous != nil {
		err = p.parseWB(input.Previous, true)
	}
	if err == nil {
		p.cumulativeOnly = false
		err = p.parseWB(input.Profile, false)
	}
	p.ctx = nil
	p.wbf = nil
	return nil
}

func (p *VTStreamingParser) parseWB(profile []byte, prev bool) (err error) {
	return decompress(profile, func(profile []byte) error {
		p.profile = profile
		p.prev = prev
		err := p.parseDecompressedWB()
		p.profile = nil
		return err
	})
}

func (p *VTStreamingParser) parseDecompressedWB() (err error) {
	if err = p.countStructs(); err != nil {
		return err
	}
	if err = p.parseFunctionsAndLocations(); err != nil {
		return err
	}
	if !p.haveKnownSampleTypes() {
		return nil
	}
	err = p.UnmarshalVTProfile(p.profile, opFlagParseSamplesWriteBatch)
	if !p.prev {
		p.wbCache.flush()
	}
	return err
}

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

	cont := p.mergeCumulativeWB()
	if !cont {
		return nil
	}
	p.appendWB()
	return nil
}

func (p *VTStreamingParser) appendWB() {
	for _, vi := range p.indexes {
		v := uint64(p.tmpSample.tmpValues[vi])
		if v == 0 {
			continue
		}
		wb := p.wbCache.getWriteBatch(p, vi)
		if wb == nil {
			continue
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
}

func (p *VTStreamingParser) mergeCumulativeWB() (cont bool) {
	if !p.cumulative {
		return true
	}

	h := p.tmpSample.hashStack(p.arena)
	for _, vi := range p.indexes {
		v := p.tmpSample.tmpValues[vi]
		if v == 0 {
			continue
		}
		wb := p.wbCache.getWriteBatch(p, vi)
		if wb == nil {
			continue
		}
		if p.prev {
			wb.prev[h] += v
		} else {
			prevV, ok := wb.prev[h]
			if ok {
				if v > prevV {
					wb.prev[h] = 0
					v -= prevV
				} else {
					wb.prev[h] = prevV - v
					v = 0
				}
				p.tmpSample.tmpValues[vi] = v
			}
		}
	}
	if p.prev {
		return false
	}
	return true
}

type writeBatchCache struct {
	// sample type -> write batch
	wbs []wbw

	cumulative bool
}

type wbw struct {
	wb        stackbuilder.WriteBatch
	appName   string
	appenders map[uint64]stackbuilder.SamplesAppender

	// stackHash -> value for cumulative prev
	prev map[uint64]int64
}

func (c *writeBatchCache) reset() {
	for i := range c.wbs {
		c.wbs[i].wb = nil
		c.wbs[i].appenders = nil
	}
}

func (c *writeBatchCache) getWriteBatch(parser *VTStreamingParser, sampleTypeIndex int) *wbw {
	if sampleTypeIndex >= len(c.wbs) {
		sz := sampleTypeIndex + 1
		if sz < 4 {
			sz = 4
		}
		newSampleTypes := arenahelper.MakeSlice[wbw](parser.arena, sz, sz)
		copy(newSampleTypes, c.wbs)
		c.wbs = newSampleTypes
	}
	p := &c.wbs[sampleTypeIndex]
	if p.wb == nil {
		appName, metadata := parser.getAppMetadata(sampleTypeIndex)
		if appName == "" {
			return nil
		}
		wb, err := parser.wbf.NewWriteBatch(appName, metadata)
		if err != nil || wb == nil {
			return nil
		}
		p.wb = wb
		p.appName = appName
		p.appenders = make(map[uint64]stackbuilder.SamplesAppender)
		if c.cumulative {
			p.prev = make(map[uint64]int64)
		}
	}
	return p
}

func (c *writeBatchCache) flush() {
	for i := range c.wbs {
		wb := c.wbs[i].wb
		if wb != nil {
			wb.Flush()
			c.wbs[i].wb = nil
			c.wbs[i].appenders = nil
		}
	}
}

func (w *wbw) getAppender(parser *VTStreamingParser, labels Labels) stackbuilder.SamplesAppender {
	h := labels.Hash()
	e, found := w.appenders[h]
	if found {
		return e
	}
	allLabels := w.resolveLabels(parser, labels)
	e = w.wb.SamplesAppender(parser.startTime.UnixNano(), parser.endTime.UnixNano(), allLabels)
	w.appenders[h] = e
	return e
}

func (w *wbw) resolveLabels(parser *VTStreamingParser, labels Labels) []stackbuilder.Label {
	labelsSize := len(parser.labels) + len(labels)
	allLabels := arenahelper.MakeSlice[stackbuilder.Label](parser.arena, 0, labelsSize)
	for k, v := range parser.labels {
		if k == "__name__" {
			v = w.appName
		}
		allLabels = append(allLabels, stackbuilder.Label{Key: k, Value: v})
	}
	for _, label := range labels {
		k := label >> 32
		if k != 0 {
			v := label & 0xffffffff
			bk := parser.string(int64(k))
			bv := parser.string(int64(v))
			//sk := ""
			//if len(bk) != 0 {
			//	//sk = unsafe.String(&bk[0], len(bk))
			//	sk = unsafe.String(&bk[0], len(bk))
			//}
			//sv := ""
			//if len(bv) != 0 {
			//	sv = unsafe.String(&bv[0], len(bv))
			//}
			sk := string(bk)
			sv := string(bv)
			allLabels = append(allLabels, stackbuilder.Label{Key: sk, Value: sv})
		}
	}
	return allLabels
}

func (s *sample) hashStack(a arenahelper.ArenaWrapper) uint64 {
	if len(s.tmpStack) == 0 {
		return zeroHash
	}
	s.stackHashes = grow(a, s.stackHashes, len(s.tmpStack))
	for _, frame := range s.tmpStack {
		h := xxhash.Sum64(frame)
		s.stackHashes = append(s.stackHashes, h)
	}

	var raw []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&raw))
	sh.Data = uintptr(unsafe.Pointer(&s.stackHashes[0]))
	sh.Len = len(s.stackHashes) * 8
	sh.Cap = len(s.stackHashes) * 8
	res := xxhash.Sum64(raw)
	return res
}
