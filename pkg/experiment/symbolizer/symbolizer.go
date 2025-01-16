package symbolizer

import (
	"context"
	"debug/dwarf"
	"debug/elf"
	"fmt"
)

// DwarfResolver implements the liner interface
type DwarfResolver struct {
	debugData *dwarf.Data
	dbgFile   *DWARFInfo
	file      *elf.File
}

func NewDwarfResolver(f *elf.File) (SymbolResolver, error) {
	debugData, err := f.DWARF()
	if err != nil {
		return nil, fmt.Errorf("read DWARF data: %w", err)
	}

	debugInfo := NewDWARFInfo(debugData)

	return &DwarfResolver{
		debugData: debugData,
		dbgFile:   debugInfo,
		file:      f,
	}, nil
}

func (d *DwarfResolver) ResolveAddress(ctx context.Context, pc uint64) ([]SymbolLocation, error) {
	return d.dbgFile.ResolveAddress(ctx, pc)
}

func (d *DwarfResolver) Close() error {
	return d.file.Close()
}

type Symbolizer struct {
	client DebuginfodClient
}

func NewSymbolizer(client DebuginfodClient) *Symbolizer {
	return &Symbolizer{
		client: client,
	}
}

func (s *Symbolizer) Symbolize(ctx context.Context, req Request) error {
	// Fetch debug info file
	debugFilePath, err := s.client.FetchDebuginfo(req.BuildID)
	if err != nil {
		return fmt.Errorf("fetch debuginfo: %w", err)
	}

	// Open ELF file
	f, err := elf.Open(debugFilePath)
	if err != nil {
		return fmt.Errorf("open ELF file: %w", err)
	}
	defer f.Close()

	// Get executable info for address normalization
	ei, err := ExecutableInfoFromELF(f)
	if err != nil {
		return fmt.Errorf("executable info from ELF: %w", err)
	}

	// Create liner
	liner, err := NewDwarfResolver(f)
	if err != nil {
		return fmt.Errorf("create liner: %w", err)
	}
	//defer liner.Close()

	// Process each mapping's locations
	for _, mapping := range req.Mappings {
		for _, loc := range mapping.Locations {
			addr, err := MapRuntimeAddress(loc.Address, ei, Mapping{
				Start:  loc.Mapping.Start,
				Limit:  loc.Mapping.Limit,
				Offset: loc.Mapping.Offset,
			})
			if err != nil {
				return fmt.Errorf("normalize address: %w", err)
			}

			// Get source lines for the address
			lines, err := liner.ResolveAddress(ctx, addr)
			if err != nil {
				continue // Skip errors for individual addresses
			}

			// Update the location directly
			loc.Lines = lines
		}
	}

	return nil
}

func (s *Symbolizer) SymbolizeAll(ctx context.Context, buildID string) error {
	// Reuse the existing debuginfo file
	debugFilePath, err := s.client.FetchDebuginfo(buildID)
	if err != nil {
		return fmt.Errorf("fetch debuginfo: %w", err)
	}

	f, err := elf.Open(debugFilePath)
	if err != nil {
		return fmt.Errorf("open ELF file: %w", err)
	}
	defer f.Close()

	debugData, err := f.DWARF()
	if err != nil {
		return fmt.Errorf("get DWARF data: %w", err)
	}

	debugInfo := NewDWARFInfo(debugData)
	allSymbols := debugInfo.SymbolizeAllAddresses()

	fmt.Println("\nSymbolizing all addresses in DWARF file:")
	fmt.Println("----------------------------------------")

	for addr, lines := range allSymbols {
		fmt.Printf("\nAddress: 0x%x\n", addr)
		for _, line := range lines {
			fmt.Printf("  Function: %s\n", line.Function.Name)
			fmt.Printf("  File: %s\n", line.Function.Filename)
			fmt.Printf("  Line: %d\n", line.Line)
			fmt.Printf("  StartLine: %d\n", line.Function.StartLine)
			fmt.Println("----------------------------------------")
		}
	}

	return nil
}
