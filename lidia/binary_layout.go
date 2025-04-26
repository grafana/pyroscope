package lidia

// BinaryLayoutInfo represents the layout information of the binary
type BinaryLayoutInfo struct {
	Type           uint16
	ProgramHeaders []ProgramHeaderInfo
}

// ProgramHeaderInfo represents a program header from the ELF file
type ProgramHeaderInfo struct {
	Type        uint32
	Flags       uint32
	Offset      uint64
	VirtualAddr uint64
	PhysAddr    uint64
	FileSize    uint64
	MemSize     uint64
	Align       uint64
}
