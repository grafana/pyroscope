package symdb

import (
	"encoding/binary"
	"slices"
	"sync"

	"github.com/cespare/xxhash/v2"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

var hashZero = xxhash.New().Sum64()

func (sm *SymbolMerger) hashMapping(h *xxhash.Digest, m *schemav1.InMemoryMapping, buf []byte) error {
	binary.LittleEndian.PutUint64(buf, m.MemoryStart)
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, m.MemoryLimit)
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sm.strings.hashes[m.Filename])
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sm.strings.hashes[m.BuildId])
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, m.FileOffset)
	if _, err := h.Write(buf); err != nil {
		return err
	}

	// Hash the boolean flags as a single byte
	var flags byte
	if m.HasFunctions {
		flags |= 1 << 0
	}
	if m.HasFilenames {
		flags |= 1 << 1
	}
	if m.HasLineNumbers {
		flags |= 1 << 2
	}
	if m.HasInlineFrames {
		flags |= 1 << 3
	}
	if _, err := h.Write([]byte{flags}); err != nil {
		return err
	}

	return nil
}

func (sm *SymbolMerger) hashFunction(h *xxhash.Digest, f *schemav1.InMemoryFunction, buf []byte) error {
	binary.LittleEndian.PutUint64(buf, sm.strings.hashes[f.Name])
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sm.strings.hashes[f.SystemName])
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sm.strings.hashes[f.Filename])
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, uint64(f.StartLine))
	if _, err := h.Write(buf); err != nil {
		return err
	}

	return nil
}

func (sm *SymbolMerger) hashLocation(h *xxhash.Digest, loc *schemav1.InMemoryLocation, buf []byte) error {
	binary.LittleEndian.PutUint64(buf, loc.Address)
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sm.mappings.hashes[loc.MappingId])
	if _, err := h.Write(buf); err != nil {
		return err
	}

	// Hash IsFolded flag
	var isFolded byte
	if loc.IsFolded {
		isFolded = 1
	}
	if _, err := h.Write([]byte{isFolded}); err != nil {
		return err
	}

	// Hash all lines
	for _, line := range loc.Line {
		binary.LittleEndian.PutUint64(buf, sm.functions.hashes[line.FunctionId])
		if _, err := h.Write(buf); err != nil {
			return err
		}
		binary.LittleEndian.PutUint64(buf, uint64(line.Line))
		if _, err := h.Write(buf); err != nil {
			return err
		}
	}

	return nil
}

func newHashedSlice[A any](equal equalityFn[A]) *hashedSlice[A] {
	return &hashedSlice[A]{
		m:     make(map[uint64]int),
		equal: equal,
	}
}

type equalityFn[A any] func(A, A) bool

type hashedSlice[A any] struct {
	m      map[uint64]int
	sl     []A
	hashes []uint64
	equal  equalityFn[A]
}

func (h *hashedSlice[A]) len() int {
	return len(h.sl)
}

func (h *hashedSlice[A]) grow(size int) {
	h.sl = slices.Grow(h.sl, size)
	h.hashes = slices.Grow(h.hashes, size)
}

func (h *hashedSlice[A]) add(hash uint64, v A) int32 {
	for probeHash := hash; ; probeHash++ {
		idx, found := h.m[probeHash]
		if !found {
			idx = len(h.sl)
			h.m[probeHash] = idx
			h.sl = append(h.sl, v)
			h.hashes = append(h.hashes, hash) // store original hash, not probe offset
			return int32(idx)
		}
		if h.equal(h.sl[idx], v) {
			return int32(idx)
		}
		// hash collision: probe next slot
	}
}

type SymbolMerger struct {
	mu sync.Mutex

	strings   *hashedSlice[string]
	mappings  *hashedSlice[schemav1.InMemoryMapping]
	functions *hashedSlice[schemav1.InMemoryFunction]
	locations *hashedSlice[schemav1.InMemoryLocation]
}

func NewSymbolMerger() *SymbolMerger {
	m := &SymbolMerger{}
	m.strings = newHashedSlice(func(a, b string) bool { return a == b })
	m.mappings = newHashedSlice(func(a, b schemav1.InMemoryMapping) bool {
		return a.MemoryStart == b.MemoryStart &&
			a.MemoryLimit == b.MemoryLimit &&
			a.FileOffset == b.FileOffset &&
			a.Filename == b.Filename &&
			a.BuildId == b.BuildId &&
			a.HasFunctions == b.HasFunctions &&
			a.HasFilenames == b.HasFilenames &&
			a.HasLineNumbers == b.HasLineNumbers &&
			a.HasInlineFrames == b.HasInlineFrames
	})
	m.functions = newHashedSlice(func(a, b schemav1.InMemoryFunction) bool {
		return a.Name == b.Name &&
			a.SystemName == b.SystemName &&
			a.Filename == b.Filename &&
			a.StartLine == b.StartLine
	})
	m.locations = newHashedSlice(func(a, b schemav1.InMemoryLocation) bool {
		if a.Address != b.Address ||
			a.MappingId != b.MappingId ||
			a.IsFolded != b.IsFolded ||
			len(a.Line) != len(b.Line) {
			return false
		}
		for i := range a.Line {
			if a.Line[i].FunctionId != b.Line[i].FunctionId ||
				a.Line[i].Line != b.Line[i].Line {
				return false
			}
		}
		return true
	})

	// make sure the first string is the empty string
	m.strings.add(hashZero, "")
	m.mappings.add(hashZero, schemav1.InMemoryMapping{})
	m.locations.add(hashZero, schemav1.InMemoryLocation{})
	m.functions.add(hashZero, schemav1.InMemoryFunction{})

	return m
}

func sortedList[A any](m map[uint32]A, lst []int32) []int32 {
	if cap(lst) < len(m) {
		lst = make([]int32, 0, len(m))
	} else {
		lst = lst[:0]
	}
	for id := range m {
		lst = append(lst, int32(id))
	}
	slices.Sort(lst)
	return lst
}

func sortedListInt32[A any](m map[int32]A, lst []int32) []int32 {
	if cap(lst) < len(m) {
		lst = make([]int32, 0, len(m))
	} else {
		lst = lst[:0]
	}
	for id := range m {
		lst = append(lst, id)
	}
	slices.Sort(lst)
	return lst
}

type remapper struct {
	mappings  map[int32]int32
	functions map[int32]int32
	strings   map[int32]int32
}

func newRemapper() *remapper {
	return &remapper{
		mappings:  make(map[int32]int32),
		functions: make(map[int32]int32),
		strings:   make(map[int32]int32),
	}
}

func (rm *remapper) resolveLocationIDs(
	locationIDs []int32,
	locations []schemav1.InMemoryLocation,
	mappings []schemav1.InMemoryMapping,
	functions []schemav1.InMemoryFunction,
) {
	// always keep empty elements
	rm.strings[0] = 0
	rm.mappings[0] = 0
	rm.functions[0] = 0

	// go through location ids collect mappings/functions that are used
	for _, locID := range locationIDs {
		rm.discoverLocation(&locations[locID])
	}

	// go through mappings collect strings used
	mappingIDs := rm.mappingIDs()
	for _, mapID := range mappingIDs {
		rm.discoverMapping(&mappings[mapID])
	}

	// go through functions collect strings used
	functionIDs := rm.functionIDs()
	for _, funcID := range functionIDs {
		rm.discoverFunction(&functions[funcID])
	}
}

func (rm *remapper) discoverLocation(loc *schemav1.InMemoryLocation) {
	rm.mappings[int32(loc.MappingId)] = -1
	for _, l := range loc.Line {
		rm.functions[int32(l.FunctionId)] = -1
	}
}

func (rm *remapper) updateLocation(loc *schemav1.InMemoryLocation) {
	loc.MappingId = uint32(rm.mappings[int32(loc.MappingId)])
	for idx := range loc.Line {
		loc.Line[idx].FunctionId = uint32(rm.functions[int32(loc.Line[idx].FunctionId)])
	}
}

func (rm *remapper) discoverMapping(mapping *schemav1.InMemoryMapping) {
	rm.strings[int32(mapping.Filename)] = -1
	rm.strings[int32(mapping.BuildId)] = -1
}

func (rm *remapper) updateMapping(mapping *schemav1.InMemoryMapping) {
	mapping.Filename = uint32(rm.strings[int32(mapping.Filename)])
	mapping.BuildId = uint32(rm.strings[int32(mapping.BuildId)])
}

func (rm *remapper) discoverFunction(function *schemav1.InMemoryFunction) {
	rm.strings[int32(function.Name)] = -1
	rm.strings[int32(function.SystemName)] = -1
	rm.strings[int32(function.Filename)] = -1
}

func (rm *remapper) updateFunction(function *schemav1.InMemoryFunction) {
	function.Name = uint32(rm.strings[int32(function.Name)])
	function.SystemName = uint32(rm.strings[int32(function.SystemName)])
	function.Filename = uint32(rm.strings[int32(function.Filename)])
}

func (rm *remapper) mappingIDs() []int32 {
	return sortedListInt32(rm.mappings, nil)
}

func (rm *remapper) functionIDs() []int32 {
	return sortedListInt32(rm.functions, nil)
}

func (rm *remapper) stringIDs() []int32 {
	return sortedListInt32(rm.strings, nil)
}

// addSymbols first determines which symbols are needed (from the locationsID slice)
func (sm *SymbolMerger) addSymbols(symbols *Symbols, locationIDs []int32) (func(model.LocationRefName) model.LocationRefName, error) {
	rm := newRemapper()

	rm.resolveLocationIDs(locationIDs, symbols.Locations, symbols.Mappings, symbols.Functions)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// add the strings to the merger
	stringIDs := rm.stringIDs()
	sm.strings.grow(len(stringIDs))
	h := xxhash.New()
	for _, sID := range stringIDs {
		s := symbols.Strings[sID]
		h.Reset()
		_, err := h.WriteString(s)
		if err != nil {
			return nil, err
		}
		rm.strings[sID] = sm.strings.add(h.Sum64(), s)
	}

	// add the functions to the merger
	var f schemav1.InMemoryFunction
	functionIDs := rm.functionIDs()
	sm.functions.grow(len(functionIDs))
	buf := make([]byte, 8)
	for _, fID := range functionIDs {
		h.Reset()
		f = symbols.Functions[fID]
		rm.updateFunction(&f)
		if err := sm.hashFunction(h, &f, buf); err != nil {
			return nil, err
		}
		rm.functions[fID] = sm.functions.add(h.Sum64(), f)
	}

	// add the mappings to the merger
	var mp schemav1.InMemoryMapping
	mappingIDs := rm.mappingIDs()
	sm.mappings.grow(len(mappingIDs))
	for _, mID := range mappingIDs {
		h.Reset()
		mp = symbols.Mappings[mID]
		rm.updateMapping(&mp)
		if err := sm.hashMapping(h, &mp, buf); err != nil {
			return nil, err
		}
		rm.mappings[mID] = sm.mappings.add(h.Sum64(), mp)
	}

	// add the locations to the merger
	var loc schemav1.InMemoryLocation
	locationRemap := make(map[int32]int32, len(locationIDs))
	for _, lID := range locationIDs {
		h.Reset()
		loc = symbols.Locations[lID]
		rm.updateLocation(&loc)
		if err := sm.hashLocation(h, &loc, buf); err != nil {
			return nil, err
		}
		locationRemap[lID] = sm.locations.add(h.Sum64(), loc)
	}

	// Return a remap function
	return func(in model.LocationRefName) model.LocationRefName {
		if in < 0 {
			return in
		}
		return model.LocationRefName(locationRemap[int32(in)])
	}, nil
}

// Add adds symbols from a TreeSymbols protobuf message to the merger.
// It returns a mapping function that can be used to remap LocationRefName values.
func (sm *SymbolMerger) Add(ts *queryv1.TreeSymbols) (func(model.LocationRefName) model.LocationRefName, error) {
	rm := newRemapper()

	// Note: We do not resolve the location IDs, as this should have happened at the previous level.
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// add the strings to the merger
	for idx, s := range ts.Strings {
		sm.strings.grow(len(ts.Strings))
		rm.strings[int32(idx)] = sm.strings.add(ts.StringHashes[idx], s)
	}

	// add the functions to the merger
	{
		var f schemav1.InMemoryFunction
		sm.functions.grow(len(ts.Functions))
		for idx, orig := range ts.Functions {
			f.StartLine = uint32(orig.StartLine)
			f.Name = uint32(orig.Name)
			f.SystemName = uint32(orig.SystemName)
			f.Filename = uint32(orig.Filename)
			rm.updateFunction(&f)
			rm.functions[int32(idx)] = sm.functions.add(ts.FunctionHashes[idx], f)
		}
	}

	// add the mappings to the merger
	{
		var m schemav1.InMemoryMapping
		sm.mappings.grow(len(ts.Mappings))
		for idx, orig := range ts.Mappings {
			m.MemoryStart = orig.MemoryStart
			m.MemoryLimit = orig.MemoryLimit
			m.FileOffset = orig.FileOffset
			m.Filename = uint32(orig.Filename)
			m.BuildId = uint32(orig.BuildId)
			m.HasFunctions = orig.HasFunctions
			m.HasFilenames = orig.HasFilenames
			m.HasLineNumbers = orig.HasLineNumbers
			m.HasInlineFrames = orig.HasInlineFrames
			rm.updateMapping(&m)
			rm.mappings[int32(idx)] = sm.mappings.add(ts.MappingHashes[idx], m)
		}
	}

	// add the locations to the merger
	var loc schemav1.InMemoryLocation
	sm.locations.grow(len(ts.Locations))
	locationRemap := make(map[int32]int32, len(ts.Locations))
	for idx, orig := range ts.Locations {
		loc.Address = orig.Address
		loc.MappingId = uint32(orig.MappingId)
		loc.IsFolded = orig.IsFolded
		loc.Line = make([]schemav1.InMemoryLine, len(orig.Line))
		for lineIdx, line := range orig.Line {
			loc.Line[lineIdx] = schemav1.InMemoryLine{
				FunctionId: uint32(line.FunctionId),
				Line:       int32(line.Line),
			}
		}
		rm.updateLocation(&loc)
		locationRemap[int32(idx)] = sm.locations.add(ts.LocationHashes[idx], loc)
	}

	// Return a remap function
	return func(in model.LocationRefName) model.LocationRefName {
		if in < 0 {
			return in
		}
		return model.LocationRefName(locationRemap[int32(in)])
	}, nil
}

// ResultBuilder creates a result builder that can be used to build the final TreeSymbols
// after all symbols have been added via Add() or addSymbols().
func (sm *SymbolMerger) ResultBuilder() *symbolResultBuilder {
	return &symbolResultBuilder{
		merger:          sm,
		locationsLookup: map[int32]int32{0: 0},
		locationsRef:    make([]int32, 1, sm.locations.len()),
	}
}

type symbolResultBuilder struct {
	merger *SymbolMerger

	locationsLookup map[int32]int32 // map from merged location index to result index
	locationsRef    []int32         // ordered list of merged location indices to include in result
}

func (m *symbolResultBuilder) KeepSymbol(in model.LocationRefName) model.LocationRefName {
	if in < 0 {
		return in
	}

	idx := int32(in)

	// Check if we've already marked this location to keep
	if resultIdx, ok := m.locationsLookup[idx]; ok {
		return model.LocationRefName(resultIdx)
	}

	// Add to the list of locations to keep
	resultIdx := int32(len(m.locationsRef))
	m.locationsLookup[idx] = resultIdx
	m.locationsRef = append(m.locationsRef, idx)
	return model.LocationRefName(resultIdx)
}

// Build constructs the final TreeSymbols protobuf message from the merged symbols.
// This should append strings, mappings, functions, locations to the TreeSymbols, ensuring via maps that each element is unique.
func (m *symbolResultBuilder) Build(r *queryv1.TreeSymbols) {
	// TODO: Quick path when nothing is truncated
	rm := newRemapper()

	if r == nil {
		r = &queryv1.TreeSymbols{}
	}

	rm.resolveLocationIDs(m.locationsRef, m.merger.locations.sl, m.merger.mappings.sl, m.merger.functions.sl)

	// add the strings to the result
	{
		stringIDs := rm.stringIDs()
		r.Strings = make([]string, len(stringIDs))
		r.StringHashes = make([]uint64, len(stringIDs))
		for idx, sID := range stringIDs {
			r.Strings[idx] = m.merger.strings.sl[sID]
			r.StringHashes[idx] = m.merger.strings.hashes[sID]
			rm.strings[sID] = int32(idx)
		}
	}

	// add the functions to the result
	{
		functionIDs := rm.functionIDs()
		functions := make([]profilev1.Function, len(functionIDs))
		r.Functions = make([]*profilev1.Function, len(functionIDs))
		r.FunctionHashes = make([]uint64, len(functionIDs))
		for idx, fID := range functionIDs {
			orig := m.merger.functions.sl[fID]
			rm.updateFunction(&orig)
			functions[idx].Id = uint64(idx)
			functions[idx].Name = int64(orig.Name)
			functions[idx].SystemName = int64(orig.SystemName)
			functions[idx].Filename = int64(orig.Filename)
			functions[idx].StartLine = int64(orig.StartLine)
			r.Functions[idx] = &functions[idx]
			r.FunctionHashes[idx] = m.merger.functions.hashes[fID]
			rm.functions[fID] = int32(idx)
		}
	}

	// add the mappings to the result
	{
		mappingIDs := rm.mappingIDs()
		mappings := make([]profilev1.Mapping, len(mappingIDs))
		r.Mappings = make([]*profilev1.Mapping, len(mappingIDs))
		r.MappingHashes = make([]uint64, len(mappingIDs))
		for idx, mID := range mappingIDs {
			orig := m.merger.mappings.sl[mID]
			rm.updateMapping(&orig)
			mappings[idx].Id = uint64(idx)
			mappings[idx].MemoryStart = orig.MemoryStart
			mappings[idx].MemoryLimit = orig.MemoryLimit
			mappings[idx].FileOffset = orig.FileOffset
			mappings[idx].Filename = int64(orig.Filename)
			mappings[idx].BuildId = int64(orig.BuildId)
			mappings[idx].HasFunctions = orig.HasFunctions
			mappings[idx].HasFilenames = orig.HasFilenames
			mappings[idx].HasLineNumbers = orig.HasLineNumbers
			mappings[idx].HasInlineFrames = orig.HasInlineFrames
			r.Mappings[idx] = &mappings[idx]
			r.MappingHashes[idx] = m.merger.mappings.hashes[mID]
			rm.mappings[mID] = int32(idx)
		}
	}

	// add the locations to the result
	{
		locations := make([]profilev1.Location, len(m.locationsRef))
		r.Locations = make([]*profilev1.Location, len(m.locationsRef))
		r.LocationHashes = make([]uint64, len(m.locationsRef))
		for idx, lID := range m.locationsRef {
			orig := m.merger.locations.sl[lID]
			rm.updateLocation(&orig)
			locations[idx].Id = uint64(idx)
			locations[idx].Address = orig.Address
			locations[idx].MappingId = uint64(orig.MappingId)
			locations[idx].IsFolded = orig.IsFolded
			locations[idx].Line = make([]*profilev1.Line, len(orig.Line))
			for lineIdx := range orig.Line {
				locations[idx].Line[lineIdx] = &profilev1.Line{
					FunctionId: uint64(orig.Line[lineIdx].FunctionId),
					Line:       int64(orig.Line[lineIdx].Line),
				}
			}
			r.Locations[idx] = &locations[idx]
			r.LocationHashes[idx] = m.merger.locations.hashes[lID]
		}
	}

}
