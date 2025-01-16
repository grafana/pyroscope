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

// DWARFInfo implements the liner interface
type DWARFInfo struct {
	debugData           *dwarf.Data
	lineEntries         map[dwarf.Offset][]dwarf.LineEntry
	subprograms         map[dwarf.Offset][]*godwarf.Tree
	abstractSubprograms map[dwarf.Offset]*dwarf.Entry
}

// NewDWARFInfo creates a new liner using DWARF debug info
func NewDWARFInfo(debugData *dwarf.Data) *DWARFInfo {
	return &DWARFInfo{
		debugData:           debugData,
		lineEntries:         make(map[dwarf.Offset][]dwarf.LineEntry),
		subprograms:         make(map[dwarf.Offset][]*godwarf.Tree),
		abstractSubprograms: make(map[dwarf.Offset]*dwarf.Entry),
	}
}

func (d *DWARFInfo) ResolveAddress(_ context.Context, addr uint64) ([]SymbolLocation, error) {
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

	file, line := d.findLineInfo(d.lineEntries[cu.Offset], targetTree.Ranges)
	lines = append(lines, SymbolLocation{
		Function: &pprof.Function{
			Name:      functionName,
			Filename:  file,
			StartLine: declLine,
		},
		Line: line,
	})

	// Enhanced inline function processing
	for _, tr := range reader.InlineStack(targetTree, addr) {

		var functionName string
		if tr.Tag == dwarf.TagSubprogram {
			functionName, ok = targetTree.Entry.Val(dwarf.AttrName).(string)
			if !ok {
				functionName = ""
			}
		} else {
			if abstractOffset, ok := tr.Entry.Val(dwarf.AttrAbstractOrigin).(dwarf.Offset); ok {
				if abstractOrigin, exists := d.abstractSubprograms[abstractOffset]; exists {
					functionName = d.getFunctionName(abstractOrigin)
				} else {
					functionName = "?"
				}
			} else {
				functionName = "?"
			}
		}

		declLine, ok := tr.Entry.Val(dwarf.AttrDeclLine).(int64)
		if !ok {
			declLine = 0
		}

		file, line := d.findLineInfo(d.lineEntries[cu.Offset], tr.Ranges)

		lines = append(lines, SymbolLocation{
			Function: &pprof.Function{
				Name:      functionName,
				Filename:  file,
				StartLine: declLine,
			},
			Line: line,
		})
	}

	return lines, nil
}

func (d *DWARFInfo) resolveFunctionName(entry *dwarf.Entry) string {
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

func (d *DWARFInfo) findLineInfo(entries []dwarf.LineEntry, ranges [][2]uint64) (string, int64) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Address < entries[j].Address
	})

	// Try to find an entry that contains our target address
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

func (d *DWARFInfo) getFunctionName(entry *dwarf.Entry) string {
	name := "?"
	ok := false
	if entry != nil {
		for _, field := range entry.Field {
			if field.Attr == dwarf.AttrName {
				name, ok = field.Val.(string)
				if !ok {
					name = "?"
				}
			}
		}
	}
	return name
}

func (d *DWARFInfo) SymbolizeAllAddresses() map[uint64][]SymbolLocation {
	results := make(map[uint64][]SymbolLocation)

	// Get all compilation units
	reader := d.debugData.Reader()
	for {
		entry, err := reader.Next()
		if err != nil || entry == nil {
			break
		}

		if entry.Tag != dwarf.TagCompileUnit {
			continue
		}

		// Get ranges for this compilation unit
		ranges, err := d.debugData.Ranges(entry)
		if err != nil {
			fmt.Printf("Warning: Failed to get ranges for CU: %v\n", err)
			continue
		}

		for _, rng := range ranges {
			// Skip invalid ranges
			if rng[0] >= rng[1] {
				continue
			}

			// Sample multiple points in this range
			addresses := []uint64{
				rng[0],                     // start
				rng[0] + (rng[1]-rng[0])/2, // middle
				rng[1] - 1,                 // end (exclusive)
			}

			for _, addr := range addresses {
				lines, err := d.ResolveAddress(context.Background(), addr)
				if err != nil {
					continue
				}

				if len(lines) > 0 {
					results[addr] = lines
				}
			}
		}
	}

	return results
}

func (d *DWARFInfo) scanAbstractSubprograms() error {
	reader := d.debugData.Reader()
	// Scan from the start, don't stop at first CU
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
	return nil
}
