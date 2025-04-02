package symbolizer

import (
	"context"
	"debug/dwarf"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/go-delve/delve/pkg/dwarf/reader"
	pprof "github.com/google/pprof/profile"
)

type FunctionInfo struct {
	Ranges    []FunctionRange
	Locations []SymbolLocation
}

type FunctionRange struct {
	StartAddr uint64
	EndAddr   uint64
}
type InlineInfo struct {
	Name      string
	File      string
	Line      int
	StartAddr uint64
	EndAddr   uint64
}

// DWARFInfo implements the liner interface
type DWARFInfo struct {
	debugData                  *dwarf.Data
	lineEntries                map[dwarf.Offset][]dwarf.LineEntry
	subprograms                map[dwarf.Offset][]*godwarf.Tree
	abstractSubprograms        map[dwarf.Offset]*dwarf.Entry
	addressMap                 map[uint64]*FunctionInfo  // Quick address lookups
	functionMap                map[string]*FunctionRange // Function name lookups
	lineEntriesSorted          map[dwarf.Offset]bool
	scannedAbstractSubprograms bool
	lastRangeInfo              *FunctionInfo
	lastRangeStart             uint64
	lastRangeEnd               uint64
}

// NewDWARFInfo creates a new liner using DWARF debug info
func NewDWARFInfo(debugData *dwarf.Data) *DWARFInfo {
	return &DWARFInfo{
		debugData:           debugData,
		lineEntries:         make(map[dwarf.Offset][]dwarf.LineEntry),
		subprograms:         make(map[dwarf.Offset][]*godwarf.Tree),
		abstractSubprograms: make(map[dwarf.Offset]*dwarf.Entry),
		addressMap:          make(map[uint64]*FunctionInfo),
		functionMap:         make(map[string]*FunctionRange),
		lineEntriesSorted:   make(map[dwarf.Offset]bool),
	}
}

func (d *DWARFInfo) ResolveAddress(_ context.Context, addr uint64) ([]SymbolLocation, error) {
	// Try optimized lookup first
	if locations, ok := d.resolveFromOptimizedMaps(addr); ok {
		return locations, nil
	}

	er := reader.New(d.debugData)
	cu, err := er.SeekPC(addr)
	if err != nil {
		return nil, fmt.Errorf("no symbol information found for address 0x%x", addr)
	}
	if cu == nil {
		return nil, errors.New("no symbol information found for address")
	}

	if err := d.buildLookupTables(cu); err != nil {
		return nil, err
	}

	var lines []SymbolLocation
	var targetTree *godwarf.Tree
	for _, tree := range d.subprograms[cu.Offset] {
		if tree.ContainsPC(addr) {
			targetTree = tree
			break
		}
	}

	if targetTree == nil {
		return lines, nil
	}

	functionName, ok := targetTree.Entry.Val(dwarf.AttrName).(string)
	if !ok {
		functionName = ""
	}

	declLine, ok := targetTree.Entry.Val(dwarf.AttrDeclLine).(int64)
	if !ok {
		declLine = 0
	}

	file, line := d.findLineInfo(d.lineEntries[cu.Offset], targetTree.Ranges, cu.Offset)
	lines = append(lines, SymbolLocation{
		Function: &pprof.Function{
			Name:      functionName,
			Filename:  file,
			StartLine: declLine,
		},
		Line: line,
	})

	inlines := d.processInlineFunctions(targetTree, addr, cu.Offset)
	for _, inline := range inlines {
		lines = append(lines, SymbolLocation{
			Function: &pprof.Function{
				Name:      inline.Name,
				Filename:  inline.File,
				StartLine: int64(inline.Line),
			},
			Line: int64(inline.Line),
		})
	}

	return lines, nil
}

func (d *DWARFInfo) buildLookupTables(cu *dwarf.Entry) error {
	// Check if we already processed this compilation unit
	if _, exists := d.lineEntries[cu.Offset]; exists {
		return nil
	}

	// TODO: not 100% sure about it. Review it.
	// Scan all DWARF entries for abstract subprograms before processing this compilation unit.
	// This scan is necessary because DWARF debug info can contain cross-compilation unit
	// references, particularly for inlined functions. When a function is inlined, its
	// definition (the abstract entry) may be in one compilation unit while its usage
	// (via AttrAbstractOrigin) can be in another. By scanning all entries upfront,
	// we ensure we can resolve these cross-unit references when they occur.
	//
	// For example, when a C++ standard library function is inlined (like printf from stdio.h),
	// its abstract entry might be in the compilation unit for stdio.h, but we need to
	// resolve its name when we find it inlined in our program's compilation unit.
	if len(d.abstractSubprograms) == 0 {
		if err := d.scanAbstractSubprograms(); err != nil {
			return fmt.Errorf("scan abstract subprograms: %w", err)
		}
	}

	// Process line entries first
	if err := d.processLineEntries(cu); err != nil {
		return fmt.Errorf("process line entries: %w", err)
	}

	// Process subprograms and their trees
	if err := d.processSubprogramEntries(cu); err != nil {
		return fmt.Errorf("process subprogram entries: %w", err)
	}

	// Build optimized maps for this CU
	if err := d.buildOptimizedMaps(cu.Offset); err != nil {
		return fmt.Errorf("build optimized maps: %w", err)
	}

	return nil
}

func (d *DWARFInfo) processLineEntries(cu *dwarf.Entry) error {
	lr, err := d.debugData.LineReader(cu)
	if err != nil {
		return fmt.Errorf("create line reader: %w", err)
	}
	if lr == nil {
		return errors.New("no line reader available")
	}

	entries := make([]dwarf.LineEntry, 0)
	for {
		var entry dwarf.LineEntry
		err := lr.Next(&entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read line entry: %w", err)
		}

		// Only store statement entries
		if entry.IsStmt {
			entries = append(entries, entry)
		}
	}

	d.lineEntries[cu.Offset] = entries
	return nil
}

func (d *DWARFInfo) processSubprogramEntries(cu *dwarf.Entry) error {
	reader := d.debugData.Reader()
	reader.Seek(cu.Offset)

	entry, err := reader.Next()
	if err != nil {
		return fmt.Errorf("read initial entry: %w", err)
	}
	if entry == nil || entry.Tag != dwarf.TagCompileUnit {
		return fmt.Errorf("unexpected entry type at CU offset: %v", cu.Offset)
	}

	subprograms := make([]*godwarf.Tree, 0)
	for {
		entry, err := reader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read entry: %w", err)
		}
		if entry == nil || entry.Tag == dwarf.TagCompileUnit {
			break
		}

		if entry.Tag != dwarf.TagSubprogram {
			continue
		}

		// Check for abstract entries first
		isAbstract := false
		for _, field := range entry.Field {
			if field.Attr == dwarf.AttrInline {
				d.abstractSubprograms[entry.Offset] = entry
				isAbstract = true
				break
			}
		}

		//Skip if this was an abstract entry
		if isAbstract {
			continue
		}

		// Extract the subprogram tree
		tree, err := godwarf.LoadTree(entry.Offset, d.debugData, 0)
		if err != nil {
			return fmt.Errorf("load subprogram tree: %w", err)
		}

		subprograms = append(subprograms, tree)
	}

	d.subprograms[cu.Offset] = subprograms
	return nil
}

func (d *DWARFInfo) findLineInfo(entries []dwarf.LineEntry, ranges [][2]uint64, cuOffset dwarf.Offset) (string, int64) {
	// Sort only once per compilation unit
	if !d.lineEntriesSorted[cuOffset] {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Address < entries[j].Address
		})
		d.lineEntriesSorted[cuOffset] = true
	}

	// Try to find an entry that contains the target address
	targetAddr := ranges[0][0]
	for _, entry := range entries {
		if entry.Address >= targetAddr && entry.Address < ranges[0][1] {
			if entry.File != nil {
				return entry.File.Name, int64(entry.Line)
			}
		}
	}

	// Find the closest entry before our target address
	var lastEntry *dwarf.LineEntry
	for i := range entries {
		if entries[i].Address > targetAddr {
			break
		}
		lastEntry = &entries[i]
	}

	if lastEntry != nil && lastEntry.File != nil {
		return lastEntry.File.Name, int64(lastEntry.Line)
	}

	return "?", 0
}

func (d *DWARFInfo) scanAbstractSubprograms() error {
	if d.scannedAbstractSubprograms {
		return nil
	}

	reader := d.debugData.Reader()
	for {
		entry, err := reader.Next()
		if err != nil || entry == nil {
			break
		}
		if entry.Tag == dwarf.TagSubprogram {
			// Store ALL subprograms, not just inline ones
			d.abstractSubprograms[entry.Offset] = entry
		}
	}

	d.scannedAbstractSubprograms = true
	return nil
}

func (d *DWARFInfo) resolveFromOptimizedMaps(addr uint64) ([]SymbolLocation, bool) {
	if info, found := d.addressMap[addr]; found {
		return info.Locations, true
	}

	// Use a simple caching mechanism for the last successful range lookup
	if d.lastRangeInfo != nil &&
		addr >= d.lastRangeStart && addr < d.lastRangeEnd {
		return d.lastRangeInfo.Locations, true
	}

	for _, info := range d.addressMap {
		for _, rang := range info.Ranges {
			if addr >= rang.StartAddr && addr < rang.EndAddr {
				d.lastRangeInfo = info
				d.lastRangeStart = rang.StartAddr
				d.lastRangeEnd = rang.EndAddr
				return info.Locations, true
			}
		}
	}

	return nil, false
}

func (d *DWARFInfo) buildOptimizedMaps(cuOffset dwarf.Offset) error {
	for _, tree := range d.subprograms[cuOffset] {
		name := d.resolveFunctionName(tree)
		if name == "?" {
			continue
		}

		if len(tree.Ranges) == 0 {
			continue
		}

		filename, line := d.findLineInfo(d.lineEntries[cuOffset], tree.Ranges, cuOffset)

		// Build locations once
		locations := []SymbolLocation{{
			Function: &pprof.Function{
				Name:      name,
				Filename:  filename,
				StartLine: int64(line),
			},
			Line: int64(line),
		}}

		// Add inline locations
		inlines := d.processInlineFunctions(tree, tree.Ranges[0][0], cuOffset)
		for _, inline := range inlines {
			locations = append(locations, SymbolLocation{
				Function: &pprof.Function{
					Name:      inline.Name,
					Filename:  inline.File,
					StartLine: int64(inline.Line),
				},
				Line: int64(inline.Line),
			})
		}

		info := &FunctionInfo{
			Ranges:    make([]FunctionRange, len(tree.Ranges)),
			Locations: locations,
		}

		for i, rang := range tree.Ranges {
			info.Ranges[i] = FunctionRange{
				StartAddr: rang[0],
				EndAddr:   rang[1],
			}
		}

		// Store in addressMap using first range start as key
		d.addressMap[tree.Ranges[0][0]] = info

		// Store in functionMap
		d.functionMap[name] = &FunctionRange{
			StartAddr: tree.Ranges[0][0],
			EndAddr:   tree.Ranges[0][1],
		}

	}
	return nil
}

func (d *DWARFInfo) processInlineFunctions(tree *godwarf.Tree, addr uint64, cuOffset dwarf.Offset) []InlineInfo {
	var inlines []InlineInfo
	for _, inline := range reader.InlineStack(tree, addr) {
		inlineName := d.resolveFunctionName(inline)
		if inlineName == "?" {
			continue
		}

		filename, line := d.findLineInfo(d.lineEntries[cuOffset], inline.Ranges, cuOffset)
		inlines = append(inlines, InlineInfo{
			Name:      inlineName,
			File:      filename,
			Line:      int(line),
			StartAddr: inline.Ranges[0][0],
			EndAddr:   inline.Ranges[0][1],
		})
	}
	return inlines
}

func (d *DWARFInfo) resolveFunctionName(entry *godwarf.Tree) string {
	if entry == nil {
		return "?"
	}

	if name, ok := entry.Val(dwarf.AttrName).(string); ok {
		return name
	}
	if name, ok := entry.Val(dwarf.AttrLinkageName).(string); ok {
		return name
	}

	return "?"
}
