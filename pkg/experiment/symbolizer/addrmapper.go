package symbolizer

import (
	"debug/elf"
	"fmt"
)

// BinaryLayout contains the information needed to translate between runtime addresses
// and the addresses in the debug information. This is necessary because:
// 1. Executables (ET_EXEC) use fixed addresses but may need segment offset adjustments
// 2. Shared libraries (ET_DYN) can be loaded at any address, requiring base address calculations
// 3. Relocatable files (ET_REL) need special handling for their relocations
type BinaryLayout struct {
	ElfType        uint16
	ProgramHeaders []MemoryRegion
}

// MemoryRegion represents a loadable segment in the ELF file.
// These segments define how the program should be loaded into memory:
// - Off: where the segment data starts in the file
// - Vaddr: the virtual address where the segment should be loaded
// - Memsz: how much memory the segment occupies when loaded
type MemoryRegion struct {
	Off    uint64 // File offset
	Vaddr  uint64 // Virtual address
	Filesz uint64 // Size in file
	Memsz  uint64 // Size in memory (may be larger than Filesz due to .bss)
	Type   uint32
}

func ExecutableInfoFromELF(f *elf.File) (*BinaryLayout, error) {
	loadableSegments := make([]MemoryRegion, 0, len(f.Progs))
	for _, segment := range f.Progs {
		if segment.Type == elf.PT_LOAD {
			loadableSegments = append(loadableSegments, MemoryRegion{
				Off:    segment.Off,
				Vaddr:  segment.Vaddr,
				Filesz: segment.Filesz,
				Memsz:  segment.Memsz,
				Type:   uint32(segment.Type),
			})
		}
	}

	return &BinaryLayout{
		ElfType:        uint16(f.Type),
		ProgramHeaders: loadableSegments,
	}, nil
}

// MapRuntimeAddress translates a runtime address to its corresponding address
// in the debug information. This translation is necessary because:
// - The program might be loaded at a different address than it was linked for
// - Different segments might need different adjustments
// - Various ELF types (EXEC, DYN, REL) handle addressing differently
func MapRuntimeAddress(runtimeAddr uint64, ei *BinaryLayout, m Mapping) (uint64, error) {
	if runtimeAddr < m.Start || runtimeAddr >= m.Limit {
		return 0, fmt.Errorf("address 0x%x out of range for mapping [0x%x-0x%x]",
			runtimeAddr, m.Start, m.Limit)
	}

	baseOffset, err := CalculateBase(ei, m, runtimeAddr)
	if err != nil {
		return runtimeAddr, fmt.Errorf("calculate base offset: %w", err)
	}

	result := runtimeAddr - baseOffset
	return result, nil
}

// CalculateBase determines the base address adjustment needed for address translation.
// The calculation varies depending on the ELF type:
// - ET_EXEC: Uses fixed addresses with potential segment adjustments
// - ET_DYN: Can be loaded anywhere, needs runtime base address adjustment
// - ET_REL: Requires relocation processing
func CalculateBase(ei *BinaryLayout, m Mapping, addr uint64) (uint64, error) {
	segment, err := ei.FindProgramHeader(m, addr)
	if err != nil {
		return 0, fmt.Errorf("find program segment: %w", err)
	}

	if segment == nil {
		return 0, nil
	}

	// Handle special case where mapping spans entire address space
	if m.Start == 0 && m.Offset == 0 && (m.Limit == ^uint64(0) || m.Limit == 0) {
		return 0, nil
	}

	switch elf.Type(ei.ElfType) {
	case elf.ET_EXEC:
		return 0, nil
	case elf.ET_DYN:
		return calculateDynamicBase(m, segment)
	}

	return 0, fmt.Errorf("unsupported ELF type: %v", elf.Type(ei.ElfType))
}

// FindProgramHeader finds the program header containing the given address.
// It returns nil if no header is found.
func (ei *BinaryLayout) FindProgramHeader(m Mapping, addr uint64) (*MemoryRegion, error) {
	// Special case: if mapping is empty (all zeros), just look for any header containing the address
	if m.Start == 0 && m.Limit == 0 {
		for i := range ei.ProgramHeaders {
			h := &ei.ProgramHeaders[i]
			if h.Type == uint32(elf.PT_LOAD) {
				if h.Vaddr <= addr && addr < h.Vaddr+h.Memsz {
					return h, nil
				}
			}
		}
		return nil, nil
	}

	// Fast path: if address is invalid or outside reasonable range
	if m.Start >= m.Limit {
		return nil, fmt.Errorf("invalid mapping range: start %x >= limit %x", m.Start, m.Limit)
	}

	// Special case: kernel addresses or very high addresses
	if m.Limit >= (1 << 63) {
		return nil, nil
	}

	// No loadable segments
	if len(ei.ProgramHeaders) == 0 {
		return nil, nil
	}

	// Calculate file offset from the address
	fileOffset := addr - m.Start + m.Offset

	// Find all headers that could contain this address
	var candidateHeaders []*MemoryRegion
	for i := range ei.ProgramHeaders {
		h := &ei.ProgramHeaders[i]
		if h.Type != uint32(elf.PT_LOAD) {
			continue
		}

		// Check if the file offset falls within this segment
		if fileOffset >= h.Off && fileOffset < h.Off+h.Memsz {
			candidateHeaders = append(candidateHeaders, h)
		}
	}

	// No matching headers found
	if len(candidateHeaders) == 0 {
		return nil, nil
	}

	// If only one header matches, return it
	if len(candidateHeaders) == 1 {
		return candidateHeaders[0], nil
	}

	// Multiple headers - need to select the most appropriate one
	// Choose the one with the closest starting address to our target
	var bestHeader *MemoryRegion
	bestDistance := uint64(^uint64(0)) // Max uint64 as initial distance

	for _, h := range candidateHeaders {
		distance := addr - h.Vaddr
		if distance < bestDistance {
			bestDistance = distance
			bestHeader = h
		}
	}

	return bestHeader, nil
}

func calculateDynamicBase(m Mapping, h *MemoryRegion) (uint64, error) {
	if h == nil {
		return m.Start - m.Offset, nil
	}

	var base uint64
	if h.Off == 0 {
		// Simple case: The segment starts at the beginning of the file
		base = m.Start - h.Vaddr
	} else {
		// Complex case: The segment starts at some offset in the file
		// Adjust for both the segment offset in the file and the virtual address
		base = m.Start - m.Offset - (h.Vaddr - h.Off)
		//base = m.Start - m.Offset + h.Off - h.Vaddr
	}

	return base, nil
}
