package streaming

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/grafana/pyroscope/pkg/og/stackbuilder"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/og/util/arenahelper"
	"github.com/valyala/bytebufferpool"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type StackFormatter int

const (
	// StackFrameFormatterGo use only function name
	StackFrameFormatterGo = 0
	// StackFrameFormatterRuby use function name, line number, function name
	StackFrameFormatterRuby = 1
)

var PPROFBufPool = bytebufferpool.Pool{}

type ParserConfig struct {
	Putter        storage.Putter
	SpyName       string
	Labels        map[string]string
	SampleTypes   map[string]*tree.SampleTypeConfig
	Formatter     StackFormatter
	ArenasEnabled bool
}

type VTStreamingParser struct {
	putter  storage.Putter
	wbf     stackbuilder.WriteBatchFactory
	spyName string
	labels  map[string]string

	sampleTypesConfig map[string]*tree.SampleTypeConfig
	Formatter         StackFormatter
	ArenasEnabled     bool

	sampleTypesFilter func(string) bool

	startTime      time.Time
	endTime        time.Time
	ctx            context.Context
	profile        []byte
	prev           bool
	cumulative     bool
	cumulativeOnly bool

	nStrings            int
	profileIDLabelIndex int64
	nFunctions          int
	nLocations          int
	nSampleTypes        int
	period              int64
	periodType          valueType
	sampleTypes         []valueType
	strings             []istr
	functions           []function
	locations           []location

	lineRefs locationFunctions

	indexes []int
	types   []int64

	tmpSample sample

	finder        finder
	previousCache LabelsCache
	newCache      LabelsCache
	wbCache       writeBatchCache
	arena         arenahelper.ArenaWrapper
}

func NewStreamingParser(config ParserConfig) *VTStreamingParser {
	res := &VTStreamingParser{}
	res.Reset(config)
	return res
}
func (p *VTStreamingParser) FreeArena() {
	arenahelper.Free(p.arena)
}
func (p *VTStreamingParser) ParsePprof(ctx context.Context, startTime, endTime time.Time, bs []byte, cumulativeOnly bool) (err error) {
	p.startTime = startTime
	p.endTime = endTime
	p.ctx = ctx
	p.cumulativeOnly = cumulativeOnly

	err = decompress(bs, func(profile []byte) error {
		p.profile = profile
		err := p.parsePprofDecompressed()
		p.profile = nil
		return err
	})
	p.ctx = nil
	return err
}

func (p *VTStreamingParser) parsePprofDecompressed() (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf(fmt.Sprintf("parse panic %s", debug.Stack()))
		}
	}()

	if err = p.countStructs(); err != nil {
		return err
	}
	if err = p.parseFunctionsAndLocations(); err != nil {
		return err
	}
	if !p.haveKnownSampleTypes() {
		return nil
	}

	p.newCache.Reset()
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
	err := p.UnmarshalVTProfile(p.profile, opFlagCountStructs)
	if err == nil {
		p.functions = grow(p.arena, p.functions, p.nFunctions)
		p.locations = grow(p.arena, p.locations, p.nLocations)
		p.strings = grow(p.arena, p.strings, p.nStrings)
		p.sampleTypes = grow(p.arena, p.sampleTypes, p.nSampleTypes)
		p.profileIDLabelIndex = 0
	}
	return err
}

func (p *VTStreamingParser) parseFunctionsAndLocations() error {
	p.lineRefs.reset(p.arena, p.nLocations)
	err := p.UnmarshalVTProfile(p.profile, opFlagParseStructs)
	if err == nil {
		p.finder = newFinder(p.functions, p.locations)
		for i := range p.sampleTypes {
			p.sampleTypes[i].resolvedType = string(p.string(p.sampleTypes[i].Type))
			p.sampleTypes[i].resolvedUnit = string(p.string(p.sampleTypes[i].unit))
		}
		p.periodType.resolvedType = string(p.string(p.periodType.Type))
		p.periodType.resolvedUnit = string(p.string(p.periodType.unit))
	}
	return err
}

func (p *VTStreamingParser) haveKnownSampleTypes() bool {
	p.indexes = grow(p.arena, p.indexes, len(p.sampleTypes))
	p.types = grow(p.arena, p.types, len(p.sampleTypes))
	for i, s := range p.sampleTypes {
		ssType := p.string(s.Type)

		st := string(ssType)
		if p.sampleTypesFilter(st) {
			if !p.cumulativeOnly || (p.cumulativeOnly && p.sampleTypesConfig[st].Cumulative) {
				p.indexes = arenahelper.AppendA(p.indexes, i, p.arena)
				p.types = arenahelper.AppendA(p.types, s.Type, p.arena)
			}
		}
	}
	if len(p.indexes) == 0 {
		return false
	}
	return true
}

func (p *VTStreamingParser) parseSamples() error {
	return p.UnmarshalVTProfile(p.profile, opFlagParseSamples)
}

func (p *VTStreamingParser) addStackLocation(lID uint64) error {
	loc, ok := p.finder.FindLocation(lID)
	if ok {
		ref := loc.linesRef
		lines := p.lineRefs.lines[(ref >> 32):(ref & 0xffffffff)]
		for i := len(lines) - 1; i >= 0; i-- {
			if err := p.addStackFrame(&lines[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *VTStreamingParser) addStackFrame(l *line) error {
	fID := l.functionID
	f, ok := p.finder.FindFunction(fID)
	if !ok {
		return nil
	}
	var frame []byte
	switch p.Formatter {
	case StackFrameFormatterRuby:
		pFuncName := p.strings[f.name]
		pFileName := p.strings[f.filename]
		frame = []byte(fmt.Sprintf("%s:%d - %s",
			p.profile[(pFileName>>32):(pFileName&0xffffffff)],
			l.line,
			p.profile[(pFuncName>>32):(pFuncName&0xffffffff)]))
	default:
	case StackFrameFormatterGo:
		pFuncName := p.strings[f.name]
		frame = p.profile[(pFuncName >> 32):(pFuncName & 0xffffffff)]
	}
	pSample := &p.tmpSample
	if len(pSample.tmpStack) < cap(pSample.tmpStack) {
		pSample.tmpStack = append(pSample.tmpStack, frame)
	} else {
		pSample.tmpStack = arenahelper.AppendA(pSample.tmpStack, frame, p.arena)
	}
	return nil
}

func (p *VTStreamingParser) string(i int64) []byte {
	ps := p.strings[i]
	return p.profile[(ps >> 32):(ps & 0xffffffff)]
}

func (p *VTStreamingParser) resolveSampleType(v int64) (*valueType, bool) {
	for i := range p.sampleTypes {
		if p.sampleTypes[i].Type == v {
			return &p.sampleTypes[i], true
		}
	}
	return nil, false
}

func (p *VTStreamingParser) iterate(fn func(stIndex int, st *valueType, l Labels, tr *tree.Tree) (keep bool, err error)) error {
	err := p.newCache.iterate(func(stIndex int, l Labels, lh uint64, tr *tree.Tree) error {
		t := &p.sampleTypes[stIndex]
		keep, err := fn(stIndex, t, l, tr)
		if err != nil {
			return err
		}
		if !keep {
			p.newCache.Remove(stIndex, lh)
		}
		return nil
	})
	if err != nil {
		return err
	}
	p.previousCache, p.newCache = p.newCache, p.previousCache
	p.newCache.Reset()
	return nil
}

func (p *VTStreamingParser) createTrees() {
	for _, vi := range p.indexes {
		v := uint64(p.tmpSample.tmpValues[vi])
		if v == 0 {
			continue
		}
		s := p.tmpSample.tmpStack
		if j := findLabelIndex(p.tmpSample.tmpLabels, p.profileIDLabelIndex); j >= 0 {
			p.newCache.GetOrCreateTree(vi, CutLabel(p.arena, p.tmpSample.tmpLabels, j)).InsertStackA(s, v)
		}
		p.newCache.GetOrCreateTree(vi, p.tmpSample.tmpLabels).InsertStackA(s, v)
	}
}

func (p *VTStreamingParser) put(stIndex int, st *valueType, l Labels, t *tree.Tree) (keep bool, err error) {
	sampleTypeBytes := st.resolvedType
	sampleType := sampleTypeBytes
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
		prev, found := p.previousCache.Get(stIndex, l.Hash())
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
		unitsBytes := st.resolvedUnit
		pi.Units = metadata.Units(unitsBytes)
		if err != nil {
			return false, err
		}
	}
	pi.Key = p.buildName(sampleType, p.ResolveLabels(l))
	err = p.putter.Put(p.ctx, &pi)
	return sampleTypeConfig.Cumulative, err
}

var vtStreamingParserPool = sync.Pool{New: func() any {
	return &VTStreamingParser{}
}}

func VTStreamingParserFromPool(config ParserConfig) *VTStreamingParser {
	res := vtStreamingParserPool.Get().(*VTStreamingParser)
	res.Reset(config)
	return res
}

func (p *VTStreamingParser) ResetCache() {
	p.previousCache.Reset()
	p.newCache.Reset()
}

func (p *VTStreamingParser) ReturnToPool() {
	if p != nil {
		vtStreamingParserPool.Put(p)
	}
}

func (p *VTStreamingParser) ResolveLabels(l Labels) map[string]string {
	m := make(map[string]string, len(l))
	for _, label := range l {
		k := label >> 32
		if k != 0 {
			v := label & 0xffffffff
			sk := p.string(int64(k))
			sv := p.string(int64(v))
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

func (p *VTStreamingParser) getAppMetadata(sampleTypeIndex int) (string, metadata.Metadata) {
	st := &p.sampleTypes[sampleTypeIndex]
	sampleType := st.resolvedType
	sampleTypeConfig, ok := p.sampleTypesConfig[sampleType]
	if !ok {
		return "", metadata.Metadata{}
	}
	if sampleTypeConfig.DisplayName != "" {
		sampleType = sampleTypeConfig.DisplayName
	}
	name := p.labels["__name__"]
	if name == "" {
		return "", metadata.Metadata{}
	}
	md := metadata.Metadata{SpyName: p.spyName}
	if sampleTypeConfig.Sampled {
		md.SampleRate = p.sampleRate()
	}
	if sampleTypeConfig.DisplayName != "" {
		sampleType = sampleTypeConfig.DisplayName
	}
	if sampleTypeConfig.Units != "" {
		md.Units = sampleTypeConfig.Units
	} else {
		// TODO(petethepig): this conversion is questionable
		unitsBytes := st.resolvedUnit
		md.Units = metadata.Units(unitsBytes)
	}
	md.AggregationType = sampleTypeConfig.Aggregation
	return name + "." + sampleType, md
}

func (p *VTStreamingParser) sampleRate() uint32 {
	if p.period <= 0 || p.periodType.unit <= 0 {
		return 0
	}
	sampleUnit := time.Nanosecond
	u := p.periodType.resolvedUnit

	switch u {
	case "microseconds":
		sampleUnit = time.Microsecond
	case "milliseconds":
		sampleUnit = time.Millisecond
	case "seconds":
		sampleUnit = time.Second
	}

	return uint32(time.Second / (sampleUnit * time.Duration(p.period)))
}

func (p *VTStreamingParser) Reset(config ParserConfig) {
	p.putter = config.Putter
	p.spyName = config.SpyName
	p.labels = config.Labels
	p.sampleTypesConfig = config.SampleTypes
	p.previousCache.Reset()
	p.newCache.Reset()
	p.wbCache.reset()

	p.sampleTypesFilter = filterKnownSamples(config.SampleTypes)
	p.Formatter = config.Formatter
	p.ArenasEnabled = config.ArenasEnabled
	if config.ArenasEnabled {
		p.arena = arenahelper.NewArenaWrapper()
		p.previousCache.arena = p.arena
		p.newCache.arena = p.arena
	}
}

func filterKnownSamples(sampleTypes map[string]*tree.SampleTypeConfig) func(string) bool {
	return func(s string) bool {
		_, ok := sampleTypes[s]
		return ok
	}
}

func findLabelIndex(tmpLabels []uint64, k int64) int {
	for i, l := range tmpLabels {
		lk := int64(l >> 32)
		if lk == k {
			return i
		}
	}
	return -1
}

func grow[T any](a arenahelper.ArenaWrapper, it []T, n int) []T {
	if it == nil || n > cap(it) {
		return arenahelper.MakeSlice[T](a, 0, n)
	}
	return it[:0]
}

func StackFrameFormatterForSpyName(spyName string) StackFormatter {
	if spyName == "rbspy" || spyName == "pyspy" {
		return StackFrameFormatterRuby
	}
	return StackFrameFormatterGo
}

func decompress(bs []byte, f func([]byte) error) error {
	var err error
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
				err = f(buf.Bytes())
			}
			PPROFBufPool.Put(buf)
			_ = gzipr.Close()
		}
	} else {
		err = f(bs)
	}
	return err
}

func stack2string(stack [][]byte, sep string) string {
	sb := strings.Builder{}
	for i, frame := range stack {
		if i != 0 {
			sb.WriteString(sep)
		}
		sb.Write(frame)
	}
	return sb.String()
}
