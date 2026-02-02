package symdb

import (
	"encoding/binary"
	"fmt"
	"iter"
	"slices"
	"sort"
	"unique"

	"github.com/cespare/xxhash/v2"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// TODO(simonswine): If successful upstream those InMemory types into schemav1
type InMemoryLine struct {
	// The id of the corresponding profile.Function for this line.
	FunctionId InlineFunction
	// Line number in source code.
	Line int32
}

type InMemoryLocation struct {
	Address   uint64
	MappingId InlineMapping
	IsFolded  bool
	Line      [8]InMemoryLine
}

var (
	emptyMapping = unique.Make(InMemoryMapping{})
	emptyString  = unique.Make("")
)

func newHashRef[A any](hashF func(*xxhash.Digest, A, []byte) error) *hashRef[A] {
	return &hashRef[A]{
		hashFunc:     hashF,
		refInSymbols: make(map[uint32]int32),
		hashes:       make([]uint64, 0),
		lst:          make([]int32, 0),
	}
}

func iterRefs[A any](refs []int32, table []A) iter.Seq2[int32, A] {
	return func(yield func(int32, A) bool) {
		for idx, ref := range refs {
			if !yield(int32(idx), (table[ref])) {
				return
			}
		}
	}
}

func iterRefsPtr[A any](refs []int32, table []A) iter.Seq2[int32, *A] {
	return func(yield func(int32, *A) bool) {
		for idx, ref := range refs {
			if !yield(int32(idx), &(table[ref])) {
				return
			}
		}
	}
}

type hashRef[A any] struct {
	hashes       []uint64         // this contains the hashes at position
	refInSymbols map[uint32]int32 // key: pos in table, value: pos in hashes
	lst          []int32          // this is the ordered list of of keys from refInSymbols

	lookupFunc func(int32) A // this looks it up in symbol
	hashFunc   func(*xxhash.Digest, A, []byte) error
}

func (hr *hashRef[A]) list() []int32 {
	if len(hr.refInSymbols) == len(hr.lst) {
		return hr.lst
	}
	hr.lst = sortedList(hr.refInSymbols, hr.lst)
	return hr.lst
}

func (hr *hashRef[A]) getHash(idx uint32) uint64 {
	hIdx, ok := hr.refInSymbols[idx]
	if !ok {
		panic("not found")
	}
	return hr.hashes[hIdx]

}

func (hr *hashRef[A]) hash(h *xxhash.Digest, b []byte, iter iter.Seq2[int32, A]) error {
	for idx, v := range iter {
		h.Reset()
		if err := hr.hashFunc(h, v, b); err != nil {
			return err
		}
		hashValue := h.Sum64()
		i := len(hr.hashes)
		hr.hashes = append(hr.hashes, hashValue)
		hr.refInSymbols[uint32(idx)] = int32(i)
	}
	return nil
}

func hashString(h *xxhash.Digest, v string, _ []byte) error {
	_, err := h.WriteString(v)
	if err != nil {
		return err
	}
	return nil
}

// this is a hashed version of Symbols
type symbolsHasher struct {
	*Symbols

	h *xxhash.Digest
	b []byte

	strings   *hashRef[string]
	mappings  *hashRef[*schemav1.InMemoryMapping]
	functions *hashRef[*schemav1.InMemoryFunction]
	locations *hashRef[*schemav1.InMemoryLocation]
}

func newSymbolsHasher(s *Symbols) *symbolsHasher {
	r := &symbolsHasher{
		h:       xxhash.New(),
		b:       make([]byte, 8),
		Symbols: s,
	}
	r.strings = newHashRef(hashString)
	r.mappings = newHashRef(r.hashMapping)
	r.functions = newHashRef(r.hashFunction)
	r.locations = newHashRef(r.hashLocation)

	return r
}

func (sym *symbolsHasher) hash() error {
	if err := sym.strings.hash(sym.h, sym.b, iterRefs(sym.strings.list(), sym.Strings)); err != nil {
		return fmt.Errorf("error hashing strings: %w", err)
	}

	if err := sym.mappings.hash(sym.h, sym.b, iterRefsPtr(sym.mappings.list(), sym.Mappings)); err != nil {
		return fmt.Errorf("error hashing strings: %w", err)
	}

	if err := sym.functions.hash(sym.h, sym.b, iterRefsPtr(sym.functions.list(), sym.Functions)); err != nil {
		return fmt.Errorf("error hashing functions: %w", err)
	}

	if err := sym.locations.hash(sym.h, sym.b, iterRefsPtr(sym.locations.list(), sym.Locations)); err != nil {
		return fmt.Errorf("error hashing locations: %w", err)
	}

	return nil
}

func (sym *symbolsHasher) hashMapping(h *xxhash.Digest, m *schemav1.InMemoryMapping, buf []byte) error {
	binary.LittleEndian.PutUint64(buf, m.MemoryStart)
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, m.MemoryLimit)
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sym.strings.getHash(m.Filename))
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sym.strings.getHash(m.BuildId))
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

func (sym *symbolsHasher) hashFunction(h *xxhash.Digest, f *schemav1.InMemoryFunction, buf []byte) error {
	binary.LittleEndian.PutUint64(buf, sym.strings.getHash(f.Name))
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sym.strings.getHash(f.SystemName))
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sym.strings.getHash(f.Filename))
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, uint64(f.StartLine))
	if _, err := h.Write(buf); err != nil {
		return err
	}

	return nil
}

func (sym *symbolsHasher) hashLocation(h *xxhash.Digest, loc *schemav1.InMemoryLocation, buf []byte) error {
	binary.LittleEndian.PutUint64(buf, loc.Address)
	if _, err := h.Write(buf); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(buf, sym.mappings.getHash(loc.MappingId))
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
		binary.LittleEndian.PutUint64(buf, sym.functions.getHash(line.FunctionId))
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

type InMemoryMapping struct {
	// Address at which the binary (or DLL) is loaded into memory.
	MemoryStart uint64
	// The limit of the address range occupied by this mapping.
	MemoryLimit uint64
	// Offset in the binary that corresponds to the first mapped address.
	FileOffset uint64
	// The object this entry is loaded from.  This can be a filename on
	// disk for the main binary and shared libraries, or virtual
	// abstractions like "[vdso]".
	Filename unique.Handle[string]
	// A string that uniquely identifies a particular program version
	// with high probability. E.g., for binaries generated by GNU tools,
	// it could be the contents of the .note.gnu.build-id field.
	BuildId unique.Handle[string]
	// The following fields indicate the resolution of symbolic info.
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

type InMemoryFunction struct {
	// Name of the function, in human-readable form if available.
	Name unique.Handle[string]
	// Name of the function, as identified by the system.
	// For instance, it can be a C++ mangled name.
	SystemName unique.Handle[string]
	// Source file containing the function.
	Filename unique.Handle[string]
	// Line number in source file.
	StartLine uint32
}

type InlineMapping = unique.Handle[InMemoryMapping]
type InlineLocation = unique.Handle[InMemoryLocation]
type InlineFunction = unique.Handle[InMemoryFunction]

func newHashedSlice[A any](merger *SymbolMerger, equal equalityFn[A]) *hashedSlice[A] {
	return &hashedSlice[A]{
		merger: merger,
		m:      make(map[uint64]int),
		equal:  equal,
	}
}

type equalityFn[A any] func(A, A, *Symbols) bool

type hashedSlice[A any] struct {
	merger *SymbolMerger
	m      map[uint64]int
	sl     []A
	equal  equalityFn[A]
}

func (h *hashedSlice[A]) len() int {
	return len(h.sl)
}

func (h *hashedSlice[A]) add(hash uint64, v A) int32 {
	idx, found := h.m[hash]
	if !found {
		idx = len(h.sl)
		h.m[hash] = idx
		h.sl = append(h.sl, v)
		return int32(idx)
	}

	if !h.equal(h.sl[idx], v, h.merger.symbols()) {
		// TODO: Handle me
		panic("hash conflict")
	}
	return int32(idx)
}

type SymbolMerger struct {
	strings   *hashedSlice[string]
	mappings  *hashedSlice[schemav1.InMemoryMapping]
	functions *hashedSlice[schemav1.InMemoryFunction]
	locations *hashedSlice[schemav1.InMemoryLocation]
}

func (m *SymbolMerger) symbols() *Symbols {
	return &Symbols{
		Locations: m.locations.sl,
		Mappings:  m.mappings.sl,
		Functions: m.functions.sl,
		Strings:   m.strings.sl,
	}
}

func NewSymbolMerger() *SymbolMerger {
	m := &SymbolMerger{}
	m.strings = newHashedSlice[string](m, func(a, b string, _ *Symbols) bool { return a == b })
	m.mappings = newHashedSlice[schemav1.InMemoryMapping](m, nil)
	m.functions = newHashedSlice[schemav1.InMemoryFunction](m, nil)
	m.locations = newHashedSlice[schemav1.InMemoryLocation](m, nil)
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

// addSymbols first determines which symbols are needed (from the locationsID slice)
// If locationIDs is nil, all locations from the symbols will be added.
func (m *SymbolMerger) addSymbols(symbols *Symbols, locationIDs []int32) func(model.LocationRefName) model.LocationRefName {
	hasher := newSymbolsHasher(symbols)

	// If no specific locations are provided, use all locations and all symbols
	addAll := locationIDs == nil
	if addAll {
		locationIDs = make([]int32, len(symbols.Locations))
		for i := range symbols.Locations {
			locationIDs[i] = int32(i)
		}
		// Also mark all mappings, functions, and strings for inclusion
		for i := range symbols.Mappings {
			hasher.mappings.refInSymbols[uint32(i)] = -1
		}
		for i := range symbols.Functions {
			hasher.functions.refInSymbols[uint32(i)] = -1
		}
		for i := range symbols.Strings {
			hasher.strings.refInSymbols[uint32(i)] = -1
		}
	}

	// go through location ids collect mappings/functions that are used
	for _, locID := range locationIDs {
		hasher.locations.refInSymbols[uint32(locID)] = -1
		if !addAll {
			loc := symbols.Locations[locID]
			hasher.mappings.refInSymbols[loc.MappingId] = -1
			for _, line := range loc.Line {
				hasher.functions.refInSymbols[line.FunctionId] = -1
			}
		}
	}

	if !addAll {
		// go through mappings collect strings used
		for _, mapID := range hasher.mappings.list() {
			m := symbols.Mappings[mapID]
			hasher.strings.refInSymbols[m.BuildId] = -1
			hasher.strings.refInSymbols[m.Filename] = -1
		}

		// go through functions collect strings used
		for _, funcID := range hasher.functions.list() {
			f := symbols.Functions[funcID]
			hasher.strings.refInSymbols[f.Filename] = -1
			hasher.strings.refInSymbols[f.Name] = -1
			hasher.strings.refInSymbols[f.SystemName] = -1
		}
	}

	// now I can hash every symbol that is used
	if err := hasher.hash(); err != nil {
		panic(err) // TODO: handle error properly
	}

	// TODO: Lock only from here

	// Add strings first and create remap table
	stringRemap := make([]uint32, len(symbols.Strings))
	for _, idx := range hasher.strings.list() {
		str := symbols.Strings[idx]
		hash := hasher.strings.getHash(uint32(idx))
		newIdx := m.strings.add(hash, str)
		stringRemap[idx] = uint32(newIdx)
	}

	// Add mappings with remapped string refs
	mappingRemap := make([]uint32, len(symbols.Mappings))
	for _, idx := range hasher.mappings.list() {
		// Make a copy to avoid modifying the original
		mapping := symbols.Mappings[idx]
		// Remap string references
		mapping.Filename = stringRemap[mapping.Filename]
		mapping.BuildId = stringRemap[mapping.BuildId]
		hash := hasher.mappings.getHash(uint32(idx))
		newIdx := m.mappings.add(hash, mapping)
		mappingRemap[idx] = uint32(newIdx)
	}

	// Add functions with remapped string refs
	functionRemap := make([]uint32, len(symbols.Functions))
	for _, idx := range hasher.functions.list() {
		// Make a copy to avoid modifying the original
		function := symbols.Functions[idx]
		// Remap string references
		function.Name = stringRemap[function.Name]
		function.SystemName = stringRemap[function.SystemName]
		function.Filename = stringRemap[function.Filename]
		hash := hasher.functions.getHash(uint32(idx))
		newIdx := m.functions.add(hash, function)
		functionRemap[idx] = uint32(newIdx)
	}

	// Add locations with remapped mapping/function refs
	locationRemap := make([]int32, len(symbols.Locations))
	for _, idx := range locationIDs {
		// Make a copy to avoid modifying the original
		location := symbols.Locations[idx]
		// Remap mapping reference
		location.MappingId = mappingRemap[location.MappingId]
		// Remap function references in lines
		for i := range location.Line {
			if location.Line[i].FunctionId != 0 {
				location.Line[i].FunctionId = functionRemap[location.Line[i].FunctionId]
			}
		}
		hash := hasher.locations.getHash(uint32(idx))
		newIdx := m.locations.add(hash, location)
		locationRemap[idx] = newIdx
	}

	// Return a remap function
	return func(in model.LocationRefName) model.LocationRefName {
		if in < 0 {
			return in
		}
		if int(in) >= len(locationRemap) {
			return in
		}
		return model.LocationRefName(locationRemap[in])
	}
}

// Add adds symbols from a TreeSymbols protobuf message to the merger.
// It returns a mapping function that can be used to remap LocationRefName values.
func (m *SymbolMerger) Add(ts *queryv1.TreeSymbols) func(model.LocationRefName) model.LocationRefName {
	// Convert TreeSymbols to Symbols
	symbols := treeSymbolsToSymbols(ts)

	// Collect all location IDs (excluding the implicit zero at index 0)
	locationIDs := make([]int32, 0, len(ts.Locations))
	for i := range ts.Locations {
		if i > 0 || (i == 0 && ts.Locations[i] != nil) {
			locationIDs = append(locationIDs, int32(i))
		}
	}

	return m.addSymbols(symbols, locationIDs)
}

// treeSymbolsToSymbols converts a TreeSymbols protobuf to internal Symbols format
func treeSymbolsToSymbols(ts *queryv1.TreeSymbols) *Symbols {
	symbols := &Symbols{
		Strings:   ts.Strings,
		Mappings:  make([]schemav1.InMemoryMapping, len(ts.Mappings)),
		Functions: make([]schemav1.InMemoryFunction, len(ts.Functions)),
		Locations: make([]schemav1.InMemoryLocation, len(ts.Locations)),
	}

	// Convert mappings
	for i, m := range ts.Mappings {
		if m != nil {
			symbols.Mappings[i] = schemav1.InMemoryMapping{
				MemoryStart:     m.MemoryStart,
				MemoryLimit:     m.MemoryLimit,
				FileOffset:      m.FileOffset,
				Filename:        uint32(m.Filename),
				BuildId:         uint32(m.BuildId),
				HasFunctions:    m.HasFunctions,
				HasFilenames:    m.HasFilenames,
				HasLineNumbers:  m.HasLineNumbers,
				HasInlineFrames: m.HasInlineFrames,
			}
		}
	}

	// Convert functions
	for i, f := range ts.Functions {
		if f != nil {
			symbols.Functions[i] = schemav1.InMemoryFunction{
				Name:       uint32(f.Name),
				SystemName: uint32(f.SystemName),
				Filename:   uint32(f.Filename),
				StartLine:  uint32(f.StartLine),
			}
		}
	}

	// Convert locations
	for i, loc := range ts.Locations {
		if loc != nil {
			inMemLoc := schemav1.InMemoryLocation{
				Address:   loc.Address,
				MappingId: uint32(loc.MappingId),
				IsFolded:  loc.IsFolded,
			}
			for j, line := range loc.Line {
				if line != nil {
					inMemLoc.Line[j] = schemav1.InMemoryLine{
						FunctionId: uint32(line.FunctionId),
						Line:       int32(line.Line),
					}
				}
			}
			symbols.Locations[i] = inMemLoc
		}
	}

	return symbols
}

// ResultBuilder creates a result builder that can be used to build the final TreeSymbols
// after all symbols have been added via Add() or addSymbols().
func (m *SymbolMerger) ResultBuilder() *symbolResultBuilder {
	return &symbolResultBuilder{
		merger:          m,
		locationsLookup: make(map[int32]int32),
		locationsRef:    make([]int32, 0, m.locations.len()),
	}
}

type lookup[K comparable] struct {
	m    map[K]int
	keys []K
}

func newLookup[K comparable](size int) *lookup[K] {
	return &lookup[K]{
		m:    make(map[K]int, size),
		keys: make([]K, 0, size),
	}
}

func (l *lookup[K]) get(v K) int {
	if idx, ok := l.m[v]; ok {
		return idx
	}

	r := len(l.keys)
	l.m[v] = r
	l.keys = append(l.keys, v)
	return r
}

func (l *lookup[K]) len() int {
	return len(l.keys)
}

func (l *lookup[K]) iter() iter.Seq2[int, K] {
	return func(yield func(int, K) bool) {
		for idx, k := range l.keys {
			if !yield(idx, k) {
				return
			}
		}
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

	// Collect all dependencies for the kept locations
	mappingRefs := make(map[uint32]int)
	functionRefs := make(map[uint32]int)
	stringRefs := make(map[uint32]int)

	// First pass: collect all referenced mappings and functions from locations
	for _, locIdx := range m.locationsRef {
		loc := m.merger.locations.sl[locIdx]
		mappingRefs[loc.MappingId] = -1
		for _, line := range loc.Line {
			if line.FunctionId != 0 {
				functionRefs[line.FunctionId] = -1
			}
		}
	}

	// Second pass: collect all referenced strings from mappings and functions
	for mappingIdx := range mappingRefs {
		mapping := m.merger.mappings.sl[mappingIdx]
		stringRefs[mapping.Filename] = -1
		stringRefs[mapping.BuildId] = -1
	}
	for functionIdx := range functionRefs {
		function := m.merger.functions.sl[functionIdx]
		stringRefs[function.Name] = -1
		stringRefs[function.SystemName] = -1
		stringRefs[function.Filename] = -1
	}

	// Build string table (ensure empty string is at index 0)
	stringsList := make([]uint32, 0, len(stringRefs))
	for strIdx := range stringRefs {
		stringsList = append(stringsList, strIdx)
	}
	sort.Slice(stringsList, func(i, j int) bool { return stringsList[i] < stringsList[j] })

	r.Strings = make([]string, 0, len(stringsList)+1)
	r.Strings = append(r.Strings, "") // Empty string at index 0
	for resultIdx, strIdx := range stringsList {
		if strIdx == 0 {
			stringRefs[strIdx] = 0
			continue
		}
		r.Strings = append(r.Strings, m.merger.strings.sl[strIdx])
		stringRefs[strIdx] = resultIdx + 1
	}

	// Build mappings table with remapped string refs
	mappingsList := make([]uint32, 0, len(mappingRefs))
	for mappingIdx := range mappingRefs {
		mappingsList = append(mappingsList, mappingIdx)
	}
	sort.Slice(mappingsList, func(i, j int) bool { return mappingsList[i] < mappingsList[j] })

	r.Mappings = make([]*googlev1.Mapping, len(mappingsList))
	for resultIdx, mappingIdx := range mappingsList {
		orig := m.merger.mappings.sl[mappingIdx]
		r.Mappings[resultIdx] = &googlev1.Mapping{
			Id:              uint64(resultIdx),
			MemoryStart:     orig.MemoryStart,
			MemoryLimit:     orig.MemoryLimit,
			FileOffset:      orig.FileOffset,
			Filename:        int64(stringRefs[orig.Filename]),
			BuildId:         int64(stringRefs[orig.BuildId]),
			HasFunctions:    orig.HasFunctions,
			HasFilenames:    orig.HasFilenames,
			HasLineNumbers:  orig.HasLineNumbers,
			HasInlineFrames: orig.HasInlineFrames,
		}
		mappingRefs[mappingIdx] = resultIdx
	}

	// Build functions table with remapped string refs
	functionsList := make([]uint32, 0, len(functionRefs))
	for functionIdx := range functionRefs {
		functionsList = append(functionsList, functionIdx)
	}
	sort.Slice(functionsList, func(i, j int) bool { return functionsList[i] < functionsList[j] })

	r.Functions = make([]*googlev1.Function, len(functionsList))
	for resultIdx, functionIdx := range functionsList {
		orig := m.merger.functions.sl[functionIdx]
		r.Functions[resultIdx] = &googlev1.Function{
			Id:         uint64(resultIdx),
			Name:       int64(stringRefs[orig.Name]),
			SystemName: int64(stringRefs[orig.SystemName]),
			Filename:   int64(stringRefs[orig.Filename]),
			StartLine:  int64(orig.StartLine),
		}
		functionRefs[functionIdx] = resultIdx
	}

	// Build locations with remapped mapping and function refs
	r.Locations = make([]*googlev1.Location, len(m.locationsRef))
	for resultIdx, locIdx := range m.locationsRef {
		orig := m.merger.locations.sl[locIdx]

		// Count non-empty lines
		lineCount := 0
		for _, line := range orig.Line {
			// Break when we find an empty line (both FunctionId and Line are 0)
			if line.FunctionId == 0 && line.Line == 0 {
				break
			}
			lineCount++
		}

		lines := make([]*googlev1.Line, lineCount)
		for i := 0; i < lineCount; i++ {
			var functionId uint64
			if orig.Line[i].FunctionId != 0 {
				functionId = uint64(functionRefs[orig.Line[i].FunctionId])
			}
			lines[i] = &googlev1.Line{
				FunctionId: functionId,
				Line:       int64(orig.Line[i].Line),
			}
		}

		r.Locations[resultIdx] = &googlev1.Location{
			Id:        uint64(resultIdx),
			MappingId: uint64(mappingRefs[orig.MappingId]),
			Address:   orig.Address,
			IsFolded:  orig.IsFolded,
			Line:      lines,
		}
	}
}
