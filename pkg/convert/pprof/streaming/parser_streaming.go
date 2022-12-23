package streaming

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/valyala/bytebufferpool"
	"io"
	"time"
)

var PPROFBufPool = bytebufferpool.Pool{}

type ParserConfig struct {
	Putter        storage.Putter
	SpyName       string
	Labels        map[string]string
	SkipExemplars bool
	SampleTypes   map[string]*tree.SampleTypeConfig
	//StackFrameFormatter StackFrameFormatter
}

type VTStreamingParser struct {
	putter            storage.Putter
	spyName           string
	labels            map[string]string
	skipExemplars     bool
	sampleTypesConfig map[string]*tree.SampleTypeConfig
	//stackFrameFormatter pprof.StackFrameFormatter

	sampleTypesFilter func(string) bool

	previousCache LabelsCache
	newCache      LabelsCache

	startTime  time.Time
	endTime    time.Time
	ctx        context.Context
	profile    []byte
	cumulative bool

	nStrings            int
	profileIDLabelIndex int64
	nFunctions          int
	nLocations          int
	period              int64
	periodType          valueType
	sampleTypes         []valueType
	strings             [][]byte
	functions           []function
	locations           []location

	indexes []int
	types   []int64

	tmpSample sample
	tmpLabel  label
	tmpLine   line

	finder finder
}

func NewStreamingParser(config ParserConfig) *VTStreamingParser {
	//if config.StackFrameFormatter == nil {//todo
	//	config.StackFrameFormatter = &pprof.UnsafeFunctionNameFormatter{}
	//}
	return &VTStreamingParser{
		putter:            config.Putter,
		spyName:           config.SpyName,
		labels:            config.Labels,
		sampleTypesConfig: config.SampleTypes,
		skipExemplars:     config.SkipExemplars,

		previousCache:     make(LabelsCache),
		sampleTypesFilter: filterKnownSamples(config.SampleTypes),
	}
}

func (p *VTStreamingParser) ParsePprof(ctx context.Context, startTime, endTime time.Time, bs []byte, cumulative bool) (err error) {
	p.startTime = startTime
	p.endTime = endTime
	p.ctx = ctx
	p.cumulative = cumulative

	if len(bs) < 2 {
		err = fmt.Errorf("failed to read pprof profile header")
	} else if bs[0] == 0x1f && bs[1] == 0x8b {
		var gzipr *gzip.Reader
		gzipr, err = gzip.NewReader(bytes.NewReader(bs))
		if err != nil {
			err = fmt.Errorf("failed to create pprof profile zip reader: %w", err)
		} else {
			buf := PPROFBufPool.Get()
			if _, err = io.Copy(buf, gzipr); err != nil {
				err = fmt.Errorf("failed to decompress gzip: %w", err)
			} else {
				p.profile = buf.Bytes()
				err = p.parsePprofDecompressed()
			}
			PPROFBufPool.Put(buf)
			_ = gzipr.Close()
		}
	} else {
		p.profile = bs
		err = p.parsePprofDecompressed()
	}
	p.ctx = nil
	p.profile = nil
	return err
}

func (p *VTStreamingParser) parsePprofDecompressed() (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("parse panic")
		}
	}()

	p.sampleTypes = make([]valueType, 0, 4)

	if err = p.countStructs(); err != nil {
		return err
	}
	if err = p.parseFunctionsAndLocations(); err != nil {
		return err
	}
	if err = p.checkKnownSampleTypes(); err != nil {
		return err
	}

	p.newCache = make(LabelsCache)
	if err = p.parseSamples(); err != nil {
		return err
	}
	return p.iterate(p.put)
}

// step 1
// - parse periodType
// - parse sampleType
// - count number of locations, functions, strings
func (p *VTStreamingParser) countStructs() error {
	err := p.UnmarshalVTStructs(p.profile)
	if err == nil {
		p.functions = make([]function, 0, p.nFunctions) //todo reuse these for consecutive parse calls? if cap is enough ?
		p.locations = make([]location, 0, p.nLocations) // reuse between parsers?
		p.strings = make([][]byte, 0, p.nStrings)
	}
	return err
}

func (p *VTStreamingParser) addString(s []byte) {
	if bytes.Equal(s, profileIDLabel) {
		p.profileIDLabelIndex = int64(len(p.strings))
	}
	p.strings = append(p.strings, s)
}

func (p *VTStreamingParser) addSampleType(st *valueType) {
	p.sampleTypes = append(p.sampleTypes, *st)
}

func (p *VTStreamingParser) addPeriodType(pt *valueType) {
	p.periodType = *pt
}

func (p *VTStreamingParser) parseFunctionsAndLocations() error {
	err := p.UnmarshalVTFunctionsAndLocations(p.profile)
	if err == nil {
		p.finder = newFinder(p.functions, p.locations)
	}
	return err
}

func (p *VTStreamingParser) addFunction(f *function) {
	p.functions = append(p.functions, *f)
}

func (p *VTStreamingParser) addLocation(l *location) {
	p.locations = append(p.locations, *l)
}

func (p *VTStreamingParser) checkKnownSampleTypes() error {
	p.indexes = make([]int, 0, len(p.sampleTypes))
	p.types = make([]int64, 0, len(p.sampleTypes))
	for i, s := range p.sampleTypes {
		ssType, err := p.string(s.Type)
		if err != nil {
			return err
		}
		st := string(ssType)
		if p.sampleTypesFilter(st) {
			if p.cumulative {
				if p.sampleTypesConfig[st].Cumulative {
					p.indexes = append(p.indexes, i)
					p.types = append(p.types, s.Type)
				}
			} else {
				p.indexes = append(p.indexes, i)
				p.types = append(p.types, s.Type)
			}
		}
	}
	if len(p.indexes) == 0 {
		return fmt.Errorf("unknown sample types")
	}
	p.tmpSample.preAllocate(len(p.indexes))
	//p.tmpValues = make([]int64, len(p.indexes))
	return nil
}

func (p *VTStreamingParser) parseSamples() error {
	return p.UnmarshalVTProfileSamples(p.profile)
	//p.mainBuf.Reset(p.profile)
	//err := molecule.MessageEach(p.mainBuf, func(field int32, value molecule.Value) (bool, error) {
	//	if profSample == field {
	//		err := p.parseSampleVT(value.Bytes)
	//		if err != nil {
	//			return false, err
	//		}
	//		//p.tmpBuf1.Reset(value.Bytes)
	//		//
	//		//err := p.parseSample(p.tmpBuf1, newCache)
	//		//if err != nil {
	//		//	return false, err
	//		//}
	//	}
	//	return true, nil
	//})
	//return err
}

func (p *VTStreamingParser) addStackLocation(lID uint64) error {
	loc, ok := p.finder.FindLocation(lID)
	if ok {
		if err := p.addStackFrame(loc.fn1); err != nil {
			return err
		}
		if err := p.addStackFrame(loc.fn2); err != nil {
			return err
		}
		if loc.extraFn != nil {
			for i := 0; i < len(loc.extraFn); i++ {
				fID := loc.extraFn[i]
				if err := p.addStackFrame(fID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (p *VTStreamingParser) addStackFrame(fID uint64) error {
	//if fID == 0 {
	if fID == noFunction {
		return nil
	}
	f, ok := p.finder.FindFunction(fID)
	if !ok {
		return nil
	}

	//name, err := p.string(f.name)
	//if err != nil {
	//	return err
	//}
	//p.tmpStack = append(p.tmpStack, name)
	p.tmpSample.tmpStack = append(p.tmpSample.tmpStack, p.strings[f.name])
	return nil
}

func (p *VTStreamingParser) string(i int64) ([]byte, error) {
	//if i < 0 || i >= len(p.strings) {
	//	return nil, fmt.Errorf("string out of bound %d", i)
	//}
	return p.strings[i], nil
}

// todo return pointer and resolve strings once
func (p *VTStreamingParser) resolveSampleType(v int64) (valueType, bool) {
	for _, vt := range p.sampleTypes {
		if vt.Type == v {
			return vt, true
		}
	}
	return valueType{}, false
}

func (p *VTStreamingParser) iterate(fn func(st valueType, l Labels, t *tree.Tree) (keep bool, err error)) error {
	for stt, entries := range p.newCache {
		t, ok := p.resolveSampleType(stt)
		if !ok {
			continue
		}

		for h, e := range entries {
			keep, err := fn(t, e.Labels, e.Tree)
			if err != nil {
				return err
			}
			if !keep {
				p.newCache.Remove(stt, h)
			}
		}
	}
	p.previousCache = p.newCache
	return nil
}

func (p *VTStreamingParser) createTrees(newCache LabelsCache) {
	for i, vi := range p.indexes {
		_ = i
		v := uint64(p.tmpSample.tmpValues[vi])
		if v == 0 {
			continue
		}
		if j := findLabelIndex(p.tmpSample.tmpLabels, p.profileIDLabelIndex); j >= 0 {
			newCache.GetOrCreateTree(p.types[i], CutLabel(p.tmpSample.tmpLabels, j)).InsertStack(p.tmpSample.tmpStack, v)
			if p.skipExemplars {
				continue
			}
		}
		newCache.GetOrCreateTree(p.types[i], p.tmpSample.tmpLabels).InsertStack(p.tmpSample.tmpStack, v)
	}
}

func (p *VTStreamingParser) put(st valueType, l Labels, t *tree.Tree) (keep bool, err error) {
	sampleTypeBytes, err := p.string(st.Type)
	if err != nil {
		return false, err
	}
	sampleType := string(sampleTypeBytes) //todo convert once
	sampleTypeConfig, ok := p.sampleTypesConfig[sampleType]
	if !ok {
		return false, fmt.Errorf("sample value type is unknown")
	}
	pi := storage.PutInput{
		StartTime: p.startTime,
		EndTime:   p.endTime,
		SpyName:   p.spyName,
		Val:       t,
	}
	// Cumulative profiles require two consecutive samples,
	// therefore we have to cache this trie.
	if sampleTypeConfig.Cumulative {
		prev, found := p.previousCache.Get(st.Type, l.Hash())
		if !found {
			// Keep the current entry in cache.
			return true, nil
		}
		// Take diff with the previous tree.
		// The result is written to prev, t is not changed.
		pi.Val = prev.Diff(t)
	}
	pi.AggregationType = sampleTypeConfig.Aggregation
	if sampleTypeConfig.Sampled {
		pi.SampleRate = p.sampleRate()
	}
	if sampleTypeConfig.DisplayName != "" {
		sampleType = sampleTypeConfig.DisplayName
	}
	if sampleTypeConfig.Units != "" {
		pi.Units = sampleTypeConfig.Units
	} else {
		// TODO(petethepig): this conversion is questionable
		unitsBytes, err := p.string(st.unit)
		pi.Units = metadata.Units(unitsBytes)
		if err != nil {
			return false, err
		}
	}
	pi.Key = p.buildName(sampleType, p.ResolveLabels(l))
	err = p.putter.Put(p.ctx, &pi)
	return sampleTypeConfig.Cumulative, err
}

func (p *VTStreamingParser) ResolveLabels(l Labels) map[string]string {
	m := make(map[string]string, len(l))
	for _, l := range l {
		if l.k != 0 {
			sk, err := p.string(l.k)
			if err != nil {
				continue
			}
			sv, err := p.string(l.v)
			if err != nil {
				continue
			}
			m[string(sk)] = string(sv)
		}
	}
	return m
}

func (p *VTStreamingParser) buildName(sampleTypeName string, labels map[string]string) *segment.Key {
	for k, v := range p.labels {
		labels[k] = v
	}
	labels["__name__"] += "." + sampleTypeName
	return segment.NewKey(labels)
}

func (p *VTStreamingParser) sampleRate() uint32 {
	if p.period <= 0 || p.periodType.unit <= 0 {
		return 0
	}
	sampleUnit := time.Nanosecond
	u, err := p.string(p.periodType.unit)
	if err == nil {
		switch string(u) { // todo convert once?
		case "microseconds":
			sampleUnit = time.Microsecond
		case "milliseconds":
			sampleUnit = time.Millisecond
		case "seconds":
			sampleUnit = time.Second
		}
	}

	return uint32(time.Second / (sampleUnit * time.Duration(p.period)))
}

func filterKnownSamples(sampleTypes map[string]*tree.SampleTypeConfig) func(string) bool {
	return func(s string) bool {
		_, ok := sampleTypes[s]
		return ok
	}
}

func findLabelIndex(tmpLabels []label, k int64) int {
	for i, l := range tmpLabels {
		if l.k == k {
			return i
		}
	}
	return -1
}
func reverseStack(s [][]byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
