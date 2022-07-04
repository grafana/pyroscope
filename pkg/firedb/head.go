package firedb

import (
	"context"
	"sync"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

type stringConversionTable []int64

func (t stringConversionTable) rewrite(idx *int64) {
	originalValue := int(*idx)
	newValue := t[originalValue]
	*idx = newValue
}

type idConversionTable map[uint64]uint64

func (t idConversionTable) rewrite(idx *uint64) {
	pos := *idx
	*idx = t[pos]
}

type Sample struct {
	// A description of the samples associated with each Sample.value.
	// For a cpu profile this might be:
	//   [["cpu","nanoseconds"]] or [["wall","seconds"]] or [["syscall","count"]]
	// For a heap profile, this might be:
	//   [["allocations","count"], ["space","bytes"]],
	// If one of the values represents the number of events represented
	// by the sample, by convention it should be at index 0 and use
	// sample_type.unit == "count".
	Types []*profilev1.ValueType `parquet:","`
	// The set of samples recorded in this profile.
	Values []*profilev1.Sample `parquet:","`
}

type stringHelper struct {
}

func (_ *stringHelper) key(s string) string {
	return s
}

func (_ *stringHelper) addToRewriter(r *rewriter, m map[int64]int64) {
	r.strings = make(stringConversionTable, len(m))
	for x, y := range m {
		r.strings[x] = y
	}
}

func (_ *stringHelper) rewrite(*rewriter, string) error {
	return nil
}

type functionsKey struct {
	Name       int64
	SystemName int64
	Filename   int64
	StartLine  int64
}

type functionHelper struct {
}

func (_ *functionHelper) key(f *profilev1.Function) functionsKey {
	return functionsKey{
		Name:       f.Name,
		SystemName: f.SystemName,
		Filename:   f.Filename,
		StartLine:  f.StartLine,
	}
}

func (_ *functionHelper) addToRewriter(r *rewriter, elemRewriter map[int64]int64) {
	r.functions = elemRewriter
}

func (_ *functionHelper) rewrite(r *rewriter, f *profilev1.Function) error {
	r.strings.rewrite(&f.Filename)
	r.strings.rewrite(&f.Name)
	r.strings.rewrite(&f.SystemName)
	return nil
}

type mappingHelper struct {
}

type mappingsKey struct {
	MemoryStart     uint64
	MemoryLimit     uint64
	FileOffset      uint64
	Filename        int64 // Index into string table
	BuildId         int64 // Index into string table
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

func (_ *mappingHelper) key(m *profilev1.Mapping) mappingsKey {
	return mappingsKey{
		MemoryStart:     m.MemoryStart,
		MemoryLimit:     m.MemoryLimit,
		FileOffset:      m.FileOffset,
		Filename:        m.Filename,
		BuildId:         m.BuildId,
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasFunctions,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func (_ *mappingHelper) addToRewriter(r *rewriter, elemRewriter map[int64]int64) {
	r.mappings = elemRewriter
}

func (_ *mappingHelper) rewrite(r *rewriter, m *profilev1.Mapping) error {
	r.strings.rewrite(&m.Filename)
	r.strings.rewrite(&m.BuildId)
	return nil
}

type Models interface {
	*profilev1.Mapping | *profilev1.Function | string
}

type Keys interface {
	mappingsKey | functionsKey | string
}

type rewriter struct {
	strings stringConversionTable

	functions map[int64]int64
	mappings  map[int64]int64
}

type Helper[M Models, K comparable] interface {
	key(M) K
	addToRewriter(*rewriter, map[int64]int64)
	rewrite(*rewriter, M) error
}

type deduplicatingSlice[M Models, K comparable, H Helper[M, K]] struct {
	slice  []M
	lock   sync.RWMutex
	lookup map[K]int64
}

func (s *deduplicatingSlice[M, K, H]) init() {
	s.lookup = make(map[K]int64)
}

func (s *deduplicatingSlice[M, K, H]) ingest(ctx context.Context, elems []M, rewriter *rewriter) error {
	var (
		missing      []int64
		rewritingMap = make(map[int64]int64)
		h            H
	)

	// rewrite elements
	for pos := range elems {
		h.rewrite(rewriter, elems[pos])
	}

	// try to find if element already exists in slice
	s.lock.RLock()
	for pos := range elems {
		k := h.key(elems[pos])
		if posSlice, exists := s.lookup[k]; exists {
			rewritingMap[int64(pos)] = posSlice
		} else {
			missing = append(missing, int64(pos))
		}
	}
	s.lock.RUnlock()

	// if there are missing elements, acquire write lock
	if len(missing) > 0 {
		s.lock.Lock()
		var posSlice = int64(len(s.slice))
		for _, pos := range missing {
			// check again if element exists
			k := h.key(elems[pos])
			if posSlice, exists := s.lookup[k]; exists {
				rewritingMap[int64(pos)] = posSlice
				continue
			}

			// add element to slice/map
			s.slice = append(s.slice, elems[pos])
			s.lookup[k] = posSlice
			rewritingMap[int64(pos)] = posSlice
			posSlice++
		}
		s.lock.Unlock()
	}

	// add rewrite information to struct
	h.addToRewriter(rewriter, rewritingMap)

	return nil
}

type Head struct {
	samples     []*Sample
	samplesLock sync.Mutex

	strings   deduplicatingSlice[string, string, *stringHelper]
	mappings  deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingHelper]
	functions deduplicatingSlice[*profilev1.Function, functionsKey, *functionHelper]
}

func NewHead() *Head {
	h := &Head{}
	h.strings.init()
	h.mappings.init()
	h.functions.init()
	return h
}

/*

// add not existing strings to the stringtable and returns a stringTableConversionTable
func (h *Head) ingestStringTable(ctx context.Context, p *profilev1.Profile) stringConversionTable {
	var (
		conversionMap = make(stringConversionTable, len(p.StringTable))
		missing       []int
	)

	// resolve existing string maps
	h.stringLock.RLock()
	for pos := range p.StringTable {
		if k, exists := h.stringMap[p.StringTable[pos]]; exists {
			conversionMap[pos] = k
		} else {
			missing = append(missing, pos)
		}
	}
	h.stringLock.RUnlock()

	// add missing strings
	if len(missing) > 0 {
		h.stringLock.Lock()
		count := int64(len(h.functions))
		for pos := range missing {
			// lookup if string is still missing now that we have the write lock
			if k, exists := h.stringMap[p.StringTable[pos]]; exists {
				conversionMap[pos] = k
				continue
			}

			// add string to string table
			h.stringTable = append(h.stringTable, p.StringTable[pos])
			h.stringMap[p.StringTable[pos]] = count
			conversionMap[pos] = count
			count++
		}
		h.stringLock.Unlock()
	}

	return conversionMap
}
*/

/*
func (h *Head) ingestFunctions(ctx context.Context, p *profilev1.Profile, strTableCnv stringConversionTable) idConversionTable {

	var (
		idConvTable = make(idConversionTable)
		missing     []int
	)

	for pos := range p.Function {
		key := functionsKeyFromFunction(p.Function[pos])
		fmt.Printf("before key=%+v\n", key)

		strTableCnv.rewrite(&p.Function[pos].Filename)
		strTableCnv.rewrite(&p.Function[pos].Name)
		strTableCnv.rewrite(&p.Function[pos].SystemName)

		key = functionsKeyFromFunction(p.Function[pos])
		fmt.Printf("after key=%+v\n", key)

		// after conversion lookup if function already exists
		h.functionsLock.RLock()
		if idx, exists := h.functionsMap[key]; exists {
			fmt.Printf("exists %+v", key)
			idConvTable[p.Function[pos].Id] = idx
		} else {
			missing = append(missing, pos)
		}
		h.functionsLock.RUnlock()
	}

	// if there were missing acquire write lock
	if len(missing) > 0 {
		h.functionsLock.Lock()
		count := uint64(len(h.functions)) + 1
		for pos := range missing {
			key := functionsKeyFromFunction(p.Function[pos])
			// check again if the function exists
			if idx, exists := h.functionsMap[key]; exists {
				idConvTable[p.Function[pos].Id] = idx
				continue
			}
			p.Function[pos].Id = count
			h.functionsMap[key] = count
			h.functions = append(h.functions, p.Function[pos])
			count++
		}
		h.functionsLock.Unlock()
	}

	return idConvTable

}

func (h *Head) ingestLocations(ctx context.Context, p *profilev1.Profile) {
}

// modifies submitted profile
func (h *Head) ingestSamples(ctx context.Context, p *profilev1.Profile, stringsConv stringConversionTable, functionsConv idConversionTable) error {
	s := &Sample{
		Types:  p.SampleType,
		Values: p.Sample,
	}

	for pos := range s.Types {
		stringsConv.rewrite(&s.Types[pos].Type)
		stringsConv.rewrite(&s.Types[pos].Unit)
	}

	for vPos := range s.Values {
		for lPos := range s.Values[vPos].Label {
			stringsConv.rewrite(&s.Values[vPos].Label[lPos].Key)
			stringsConv.rewrite(&s.Values[vPos].Label[lPos].Str)
			stringsConv.rewrite(&s.Values[vPos].Label[lPos].NumUnit)
		}
	}

	h.samplesLock.Lock()
	h.samples = append(h.samples, s)
	h.samplesLock.Unlock()

	return nil
}
*/

func (h *Head) Ingest(ctx context.Context, p *profilev1.Profile) error {
	rewrites := &rewriter{}

	if err := h.strings.ingest(ctx, p.StringTable, rewrites); err != nil {
		return err
	}

	if err := h.mappings.ingest(ctx, p.Mapping, rewrites); err != nil {
		return err
	}

	if err := h.functions.ingest(ctx, p.Function, rewrites); err != nil {
		return err
	}

	return nil
}
