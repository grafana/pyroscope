package streaming

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/stackbuilder"
	"github.com/pyroscope-io/pyroscope/pkg/util/arenahelper"
	"time"
)

func (p *VTStreamingParser) ParseWithWriteBatch(ctx context.Context, startTime, endTime time.Time, compressedProfile []byte, wbf stackbuilder.WriteBatchFactory) (err error) {
	//defer func() {
	//	if recover() != nil {
	//		err = fmt.Errorf(fmt.Sprintf("parse panic %s", debug.Stack()))
	//	}
	//}()
	p.startTime = startTime
	p.endTime = endTime
	p.ctx = ctx
	p.wbf = wbf
	err = decompress(compressedProfile, func(profile []byte) error {
		p.profile = profile
		err := p.parseWB()
		p.profile = nil
		return err
	})
	p.ctx = nil
	p.wbf = nil
	return nil
}

func (p *VTStreamingParser) parseWB() (err error) {
	if err = p.countStructs(); err != nil {
		return err
	}
	if err = p.parseFunctionsAndLocations(); err != nil {
		return err
	}
	if !p.haveKnownSampleTypes() {
		return nil
	}
	return p.parseSamplesWB()
}

func (p *VTStreamingParser) parseSamplesWB() error {
	err := p.UnmarshalVTProfile(p.profile, opFlagParseSamplesWriteBatch)
	p.wbCache.flush()
	return err
}

type writeBatchCache struct {
	// sample type -> write batch
	wbs []wbw

	appName string //todo keep it in parser
}

type wbw struct {
	wb        stackbuilder.WriteBatch
	appenders map[uint64]stackbuilder.SamplesAppender
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
		sampleType := parser.sampleTypes[sampleTypeIndex].resolvedType
		sampleTypeConfig, ok := parser.sampleTypesConfig[sampleType]
		if !ok {
			return nil
		}
		if sampleTypeConfig.DisplayName != "" {
			sampleType = sampleTypeConfig.DisplayName
		}
		appName := c.appName + "." + sampleType
		wb, err := parser.wbf.NewWriteBatch(appName)
		if err != nil || wb == nil {
			return nil
		}
		p.wb = wb
		p.appenders = make(map[uint64]stackbuilder.SamplesAppender)
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
	h := labels.Hash() //todo collisions?
	e, found := w.appenders[h]
	if found {
		return e
	}
	allLabels := resolveLabels(parser, labels)
	e = w.wb.SamplesAppender(parser.startTime.Unix(), parser.endTime.Unix(), allLabels)
	w.appenders[h] = e
	return e
}

func resolveLabels(parser *VTStreamingParser, labels Labels) []stackbuilder.Label {
	labelsSize := len(parser.labels) + len(labels) - 1
	allLabels := arenahelper.MakeSlice[stackbuilder.Label](parser.arena, 0, labelsSize)
	for k, v := range parser.labels {
		if k == "__name__" {
			continue
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
