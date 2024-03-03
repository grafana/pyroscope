package bininspect

import (
	"debug/dwarf"
	"fmt"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/go-delve/delve/pkg/dwarf/loclist"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore/dd/dwarfutils"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore/dd/dwarfutils/locexpr"
)

// dwarfInspector is used to keep common data for the dwarf inspection functions.
type DwarfInspector struct {
	elf       ElfMetadata
	dwarfData *dwarf.Data
}

func NewDwarfInspector(elf *ElfMetadata, dwarfData *dwarf.Data) *DwarfInspector {
	return &DwarfInspector{
		elf:       *elf,
		dwarfData: dwarfData,
	}
}

func (d DwarfInspector) GetParameterLocationAtPC(parameterDIE *dwarf.Entry, pc uint64) (ParameterMetadata, error) {
	typeOffset, ok := parameterDIE.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return ParameterMetadata{}, fmt.Errorf("no type offset attribute in parameter entry")
	}

	// Find the location field on the entry
	locationField := parameterDIE.AttrField(dwarf.AttrLocation)
	if locationField == nil {
		return ParameterMetadata{}, fmt.Errorf("no location field in parameter entry")
	}

	typ, err := dwarfutils.NewTypeFinder(d.dwarfData).FindTypeByOffset(typeOffset)
	if err != nil {
		return ParameterMetadata{}, fmt.Errorf("could not find parameter type by offset: %w", err)
	}

	// The location field can be one of two things:
	// (See DWARF v4 spec section 2.6)
	// 1. Single location descriptions,
	//    which specifies a location expression as the direct attribute value.
	//    This has a DWARF class of `exprloc`,
	//    and the value is a `[]byte` that can be directly interpreted.
	// 2. Location lists, which gives an index into the loclists section.
	//    This has a DWARF class of `loclistptr`,
	//    which is used to index into the location list
	//    and to get the location expression that corresponds to
	//    the given program counter
	//    (in this case, that is the entry of the function, where we will attach the uprobe).
	var locationExpression []byte
	switch locationField.Class {
	case dwarf.ClassExprLoc:
		if locationValAsBytes, ok := locationField.Val.([]byte); ok {
			locationExpression = locationValAsBytes
		} else {
			return ParameterMetadata{}, fmt.Errorf("formal parameter entry contained invalid value for location attribute: locationField=%#v", locationField)
		}
	case dwarf.ClassLocListPtr:
		locationAsLocListIndex, ok := locationField.Val.(int64)
		if !ok {
			return ParameterMetadata{}, fmt.Errorf("could not interpret location attribute in formal parameter entry as location list pointer: locationField=%#v", locationField)
		}

		loclistEntry, err := d.getLoclistEntry(locationAsLocListIndex, pc)
		if err != nil {
			return ParameterMetadata{}, fmt.Errorf("could not find loclist entry at %#x for PC %#x: %w", locationAsLocListIndex, pc, err)
		}
		locationExpression = loclistEntry.Instr
	default:
		return ParameterMetadata{}, fmt.Errorf("unexpected field class on formal parameter's location attribute: locationField=%#v", locationField)
	}

	totalSize := typ.Size()
	pieces, err := locexpr.Exec(locationExpression, totalSize, int(d.elf.Arch.PointerSize()))
	if err != nil {
		return ParameterMetadata{}, fmt.Errorf("error executing location expression for parameter: %w", err)
	}
	inspectPieces := make([]ParameterPiece, len(pieces))
	for i, piece := range pieces {
		inspectPieces[i] = ParameterPiece{
			Size:        piece.Size,
			InReg:       piece.InReg,
			StackOffset: piece.StackOffset,
			Register:    piece.Register,
		}
	}
	return ParameterMetadata{
		TotalSize: totalSize,
		Kind:      typ.Common().ReflectKind,
		Pieces:    inspectPieces,
	}, nil
}

// getLoclistEntry returns the loclist entry in the loclist
// starting at offset, for address pc.
// Adapted from github.com/go-delve/delve/pkg/proc.(*BinaryInfo).loclistEntry
func (d DwarfInspector) getLoclistEntry(offset int64, pc uint64) (*loclist.Entry, error) {
	debugInfoBytes, err := godwarf.GetDebugSectionElf(d.elf.File, "info")
	if err != nil {
		return nil, err
	}

	compileUnits, err := dwarfutils.LoadCompileUnits(d.dwarfData, debugInfoBytes)
	if err != nil {
		return nil, err
	}

	debugLocBytes, _ := godwarf.GetDebugSectionElf(d.elf.File, "loc")
	loclist2 := loclist.NewDwarf2Reader(debugLocBytes, int(d.elf.Arch.PointerSize()))
	debugLoclistBytes, _ := godwarf.GetDebugSectionElf(d.elf.File, "loclists")
	loclist5 := loclist.NewDwarf5Reader(debugLoclistBytes)
	debugAddrBytes, _ := godwarf.GetDebugSectionElf(d.elf.File, "addr")
	debugAddrSection := godwarf.ParseAddr(debugAddrBytes)

	var base uint64
	compileUnit := compileUnits.FindCompileUnit(pc)
	if compileUnit != nil {
		base = compileUnit.LowPC
	}

	var loclist loclist.Reader = loclist2
	var debugAddr *godwarf.DebugAddr
	if compileUnit != nil && compileUnit.Version >= 5 && loclist5 != nil {
		loclist = loclist5
		if addrBase, ok := compileUnit.Entry.Val(dwarf.AttrAddrBase).(int64); ok {
			debugAddr = debugAddrSection.GetSubsection(uint64(addrBase))
		}
	}

	if loclist.Empty() {
		return nil, fmt.Errorf("no loclist found for the given program counter")
	}

	// Use 0x0 as the static base
	var staticBase uint64 = 0x0
	entry, err := loclist.Find(int(offset), staticBase, base, pc, debugAddr)
	if err != nil {
		return nil, fmt.Errorf("error reading loclist section: %w", err)
	}
	if entry != nil {
		return entry, nil
	}

	return nil, fmt.Errorf("no loclist entry found")
}
