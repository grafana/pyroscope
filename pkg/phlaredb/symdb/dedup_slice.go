//nolint:unused,unparam
package symdb

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"sort"
	"sync"
	"unsafe"

	"github.com/colega/zeropool"
	"go.uber.org/atomic"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/slices"
)

// Refactored as is from the phlaredb package.

var (
	int64SlicePool  zeropool.Pool[[]int64]
	uint32SlicePool zeropool.Pool[[]uint32]
)

// TODO(kolesnikovae):
//   - PartitionWriter should only rewrite profile symbol indices;
//   - InMemoryProfile should be created somewhere else on the call side.

func (p *PartitionWriter) WriteProfileSymbols(profile *profilev1.Profile) []schemav1.InMemoryProfile {
	// create a rewriter state
	rewrites := &rewriter{}

	spans := pprof.ProfileSpans(profile)
	pprof.ZeroLabelStrings(profile)

	p.strings.ingest(profile.StringTable, rewrites)
	mappings := make([]*schemav1.InMemoryMapping, len(profile.Mapping))
	for i, v := range profile.Mapping {
		mappings[i] = &schemav1.InMemoryMapping{
			Id:              v.Id,
			MemoryStart:     v.MemoryStart,
			MemoryLimit:     v.MemoryLimit,
			FileOffset:      v.FileOffset,
			Filename:        uint32(v.Filename),
			BuildId:         uint32(v.BuildId),
			HasFunctions:    v.HasFunctions,
			HasFilenames:    v.HasFilenames,
			HasLineNumbers:  v.HasLineNumbers,
			HasInlineFrames: v.HasInlineFrames,
		}
	}

	p.mappings.ingest(mappings, rewrites)
	funcs := make([]*schemav1.InMemoryFunction, len(profile.Function))
	for i, v := range profile.Function {
		funcs[i] = &schemav1.InMemoryFunction{
			Id:         v.Id,
			Name:       uint32(v.Name),
			SystemName: uint32(v.SystemName),
			Filename:   uint32(v.Filename),
			StartLine:  uint32(v.StartLine),
		}
	}

	p.functions.ingest(funcs, rewrites)
	locs := make([]*schemav1.InMemoryLocation, len(profile.Location))
	for i, v := range profile.Location {
		x := &schemav1.InMemoryLocation{
			Id:        v.Id,
			Address:   v.Address,
			MappingId: uint32(v.MappingId),
			IsFolded:  v.IsFolded,
		}
		x.Line = make([]schemav1.InMemoryLine, len(v.Line))
		for j, line := range v.Line {
			x.Line[j] = schemav1.InMemoryLine{
				FunctionId: uint32(line.FunctionId),
				Line:       int32(line.Line),
			}
		}
		locs[i] = x
	}

	p.locations.ingest(locs, rewrites)
	samplesPerType := p.convertSamples(rewrites, profile.Sample, spans)

	profiles := make([]schemav1.InMemoryProfile, len(samplesPerType))
	for idxType := range samplesPerType {
		profiles[idxType] = schemav1.InMemoryProfile{
			StacktracePartition: p.header.Partition,
			Samples:             samplesPerType[idxType],
			DropFrames:          profile.DropFrames,
			KeepFrames:          profile.KeepFrames,
			TimeNanos:           profile.TimeNanos,
			DurationNanos:       profile.DurationNanos,
			Comments:            copySlice(profile.Comment),
			DefaultSampleType:   profile.DefaultSampleType,
		}
	}

	return profiles
}

func (p *PartitionWriter) convertSamples(r *rewriter, in []*profilev1.Sample, spans []uint64) []schemav1.Samples {
	if len(in) == 0 {
		return nil
	}

	// populate output
	var (
		samplesByType = make([]schemav1.Samples, len(in[0].Value))
		stacktraces   = make([]*schemav1.Stacktrace, len(in))
	)

	for i := range samplesByType {
		s := schemav1.Samples{
			Values:        make([]uint64, len(in)),
			StacktraceIDs: make([]uint32, len(in)),
		}
		if len(spans) > 0 {
			s.Spans = make([]uint64, len(spans))
			copy(s.Spans, spans)
		}
		samplesByType[i] = s
	}

	for idxSample := range in {
		// populate samples
		src := in[idxSample]
		for idxType := range samplesByType {
			samplesByType[idxType].Values[idxSample] = uint64(src.Value[idxType])
		}
		stacktraces[idxSample] = &schemav1.Stacktrace{LocationIDs: src.LocationId}
		for i := range stacktraces[idxSample].LocationIDs {
			r.locations.rewriteUint64(&stacktraces[idxSample].LocationIDs[i])
		}
	}

	stacktracesIds := slices.GrowLen(uint32SlicePool.Get(), len(stacktraces))
	p.stacktraces.append(stacktracesIds, stacktraces)

	// Rewrite stacktraces
	for idxType := range samplesByType {
		samples := samplesByType[idxType]
		for i := range samples.StacktraceIDs {
			samples.StacktraceIDs[i] = stacktracesIds[i]
		}
		samples = samples.Compact(false)
		sort.Sort(samples)
		samplesByType[idxType] = samples
	}

	uint32SlicePool.Put(stacktracesIds)
	return samplesByType
}

func copySlice[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

type idConversionTable map[int64]int64

// nolint unused
func (t idConversionTable) rewrite(idx *int64) {
	pos := *idx
	var ok bool
	*idx, ok = t[pos]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
}

// nolint unused
func (t idConversionTable) rewriteUint64(idx *uint64) {
	pos := *idx
	v, ok := t[int64(pos)]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
	*idx = uint64(v)
}

// nolint unused
func (t idConversionTable) rewriteUint32(idx *uint32) {
	pos := *idx
	v, ok := t[int64(pos)]
	if !ok {
		panic(fmt.Sprintf("unable to rewrite index %d", pos))
	}
	*idx = uint32(v)
}

func emptyRewriter() *rewriter {
	return &rewriter{
		strings: []int64{0},
	}
}

// rewriter contains slices to rewrite the per profile reference into per head references.
type rewriter struct {
	strings stringConversionTable
	// nolint unused
	functions idConversionTable
	// nolint unused
	mappings idConversionTable
	// nolint unused
	locations idConversionTable
}

type storeHelper[M schemav1.Models] interface {
	// some Models contain their own IDs within the struct, this allows to set them and keep track of the preexisting ID. It should return the oldID that is supposed to be rewritten.
	setID(existingSliceID uint64, newID uint64, element M) uint64

	// size returns a (rough estimation) of the size of a single element M
	size(M) uint64

	// clone copies parts that are not optimally sized from protobuf parsing
	clone(M) M

	rewrite(*rewriter, M) error
}

type Helper[M schemav1.Models, K comparable] interface {
	storeHelper[M]
	key(M) K
	addToRewriter(*rewriter, idConversionTable)
}

type deduplicatingSlice[M schemav1.Models, K comparable, H Helper[M, K]] struct {
	lock   sync.RWMutex
	slice  []M
	size   atomic.Uint64
	lookup map[K]int64

	helper H
}

func (s *deduplicatingSlice[M, K, H]) init() {
	s.lookup = make(map[K]int64)
}

func (s *deduplicatingSlice[M, K, H]) MemorySize() uint64 {
	// FIXME(kolesnikovae): Map footprint + slice capacity.
	return s.size.Load()
}

func (s *deduplicatingSlice[M, K, H]) Size() uint64 {
	return s.size.Load()
}

func (s *deduplicatingSlice[M, K, H]) ingest(elems []M, rewriter *rewriter) {
	var (
		rewritingMap = make(map[int64]int64)
		missing      = int64SlicePool.Get()
	)
	missing = missing[:0]
	// rewrite elements
	for pos := range elems {
		_ = s.helper.rewrite(rewriter, elems[pos])
	}

	// try to find if element already exists in slice, when supposed to deduplicate
	s.lock.RLock()
	for pos := range elems {
		k := s.helper.key(elems[pos])
		if posSlice, exists := s.lookup[k]; exists {
			rewritingMap[int64(s.helper.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
		} else {
			missing = append(missing, int64(pos))
		}
	}
	s.lock.RUnlock()

	// if there are missing elements, acquire write lock
	if len(missing) > 0 {
		s.lock.Lock()
		posSlice := int64(len(s.slice))
		for _, pos := range missing {
			// check again if element exists
			k := s.helper.key(elems[pos])
			if posSlice, exists := s.lookup[k]; exists {
				rewritingMap[int64(s.helper.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
				continue
			}

			// add element to slice/map
			s.slice = append(s.slice, s.helper.clone(elems[pos]))
			s.lookup[k] = posSlice
			rewritingMap[int64(s.helper.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
			posSlice++
			s.size.Add(s.helper.size(elems[pos]))
		}
		s.lock.Unlock()
	}

	// nolint staticcheck
	int64SlicePool.Put(missing)

	// add rewrite information to struct
	s.helper.addToRewriter(rewriter, rewritingMap)
}

func (s *deduplicatingSlice[M, K, H]) append(dst []uint32, elems []M) {
	missing := int64SlicePool.Get()[:0]
	s.lock.RLock()
	for i, v := range elems {
		k := s.helper.key(v)
		if x, ok := s.lookup[k]; ok {
			dst[i] = uint32(x)
		} else {
			missing = append(missing, int64(i))
		}
	}
	s.lock.RUnlock()
	if len(missing) > 0 {
		s.lock.RLock()
		p := uint32(len(s.slice))
		for _, i := range missing {
			e := elems[i]
			k := s.helper.key(e)
			x, ok := s.lookup[k]
			if ok {
				dst[i] = uint32(x)
				continue
			}
			s.size.Add(s.helper.size(e))
			s.slice = append(s.slice, s.helper.clone(e))
			s.lookup[k] = int64(p)
			dst[i] = p
			p++
		}
		s.lock.RUnlock()
	}
	int64SlicePool.Put(missing)
}

func (s *deduplicatingSlice[M, K, H]) sliceHeaderCopy() []M {
	s.lock.RLock()
	h := s.slice
	s.lock.RUnlock()
	return h
}

type stringConversionTable []int64

func (t stringConversionTable) rewrite(idx *int64) {
	originalValue := int(*idx)
	newValue := t[originalValue]
	*idx = newValue
}

func (t stringConversionTable) rewriteUint32(idx *uint32) {
	originalValue := int(*idx)
	newValue := t[originalValue]
	*idx = uint32(newValue)
}

type stringsHelper struct{}

func (*stringsHelper) key(s string) string {
	return s
}

func (*stringsHelper) addToRewriter(r *rewriter, m idConversionTable) {
	var maxID int64
	for id := range m {
		if id > maxID {
			maxID = id
		}
	}
	r.strings = make(stringConversionTable, maxID+1)

	for x, y := range m {
		r.strings[x] = y
	}
}

// nolint unused
func (*stringsHelper) rewrite(*rewriter, string) error {
	return nil
}

func (*stringsHelper) size(s string) uint64 {
	return uint64(len(s))
}

func (*stringsHelper) setID(oldID, newID uint64, s string) uint64 {
	return oldID
}

func (*stringsHelper) clone(s string) string {
	return s
}

type locationsKey struct {
	MappingId uint32 //nolint
	Address   uint64
	LinesHash uint64
}

const (
	lineSize     = uint64(unsafe.Sizeof(schemav1.InMemoryLine{}))
	locationSize = uint64(unsafe.Sizeof(schemav1.InMemoryLocation{}))
)

type locationsHelper struct{}

func (*locationsHelper) key(l *schemav1.InMemoryLocation) locationsKey {
	return locationsKey{
		Address:   l.Address,
		MappingId: l.MappingId,
		LinesHash: hashLines(l.Line),
	}
}

var mapHashSeed = maphash.MakeSeed()

func hashLines(s []schemav1.InMemoryLine) uint64 {
	if len(s) == 0 {
		return 0
	}
	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Len = len(s) * int(lineSize)
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&s[0]))
	return maphash.Bytes(mapHashSeed, b)
}

func hashLocations(s []uint64) uint64 {
	if len(s) == 0 {
		return 0
	}
	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Len = len(s) * 8
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&s[0]))
	return maphash.Bytes(mapHashSeed, b)
}

func (*locationsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*locationsHelper) rewrite(r *rewriter, l *schemav1.InMemoryLocation) error {
	// when mapping id is not 0, rewrite it
	if l.MappingId != 0 {
		r.mappings.rewriteUint32(&l.MappingId)
	}
	for pos := range l.Line {
		r.functions.rewriteUint32(&l.Line[pos].FunctionId)
	}
	return nil
}

func (*locationsHelper) setID(_, newID uint64, l *schemav1.InMemoryLocation) uint64 {
	oldID := l.Id
	l.Id = newID
	return oldID
}

func (*locationsHelper) size(l *schemav1.InMemoryLocation) uint64 {
	return uint64(len(l.Line))*lineSize + locationSize
}

func (*locationsHelper) clone(l *schemav1.InMemoryLocation) *schemav1.InMemoryLocation {
	x := *l
	x.Line = make([]schemav1.InMemoryLine, len(l.Line))
	copy(x.Line, l.Line)
	return &x
}

type mappingsHelper struct{}

const mappingSize = uint64(unsafe.Sizeof(schemav1.InMemoryMapping{}))

type mappingsKey struct {
	MemoryStart     uint64
	MemoryLimit     uint64
	FileOffset      uint64
	Filename        uint32 // Index into string table
	BuildId         uint32 // Index into string table
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

func (*mappingsHelper) key(m *schemav1.InMemoryMapping) mappingsKey {
	return mappingsKey{
		MemoryStart:     m.MemoryStart,
		MemoryLimit:     m.MemoryLimit,
		FileOffset:      m.FileOffset,
		Filename:        m.Filename,
		BuildId:         m.BuildId,
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasLineNumbers,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func (*mappingsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.mappings = elemRewriter
}

// nolint unparam
func (*mappingsHelper) rewrite(r *rewriter, m *schemav1.InMemoryMapping) error {
	r.strings.rewriteUint32(&m.Filename)
	r.strings.rewriteUint32(&m.BuildId)
	return nil
}

func (*mappingsHelper) setID(_, newID uint64, m *schemav1.InMemoryMapping) uint64 {
	oldID := m.Id
	m.Id = newID
	return oldID
}

func (*mappingsHelper) size(_ *schemav1.InMemoryMapping) uint64 {
	return mappingSize
}

func (*mappingsHelper) clone(m *schemav1.InMemoryMapping) *schemav1.InMemoryMapping {
	x := *m
	return &x
}

type functionsKey struct {
	Name       uint32
	SystemName uint32
	Filename   uint32
	StartLine  uint32
}

type functionsHelper struct{}

const functionSize = uint64(unsafe.Sizeof(schemav1.InMemoryFunction{}))

func (*functionsHelper) key(f *schemav1.InMemoryFunction) functionsKey {
	return functionsKey{
		Name:       f.Name,
		SystemName: f.SystemName,
		Filename:   f.Filename,
		StartLine:  f.StartLine,
	}
}

func (*functionsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.functions = elemRewriter
}

func (*functionsHelper) rewrite(r *rewriter, f *schemav1.InMemoryFunction) error {
	r.strings.rewriteUint32(&f.Filename)
	r.strings.rewriteUint32(&f.Name)
	r.strings.rewriteUint32(&f.SystemName)
	return nil
}

func (*functionsHelper) setID(_, newID uint64, f *schemav1.InMemoryFunction) uint64 {
	oldID := f.Id
	f.Id = newID
	return oldID
}

func (*functionsHelper) size(_ *schemav1.InMemoryFunction) uint64 {
	return functionSize
}

func (*functionsHelper) clone(f *schemav1.InMemoryFunction) *schemav1.InMemoryFunction {
	x := *f
	return &x
}
