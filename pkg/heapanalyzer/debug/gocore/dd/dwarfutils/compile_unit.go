// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package dwarfutils provides utilities for working with DWARF debug information.
package dwarfutils

import (
	"debug/dwarf"

	"github.com/go-delve/delve/pkg/dwarf/util"
)

// CompileUnits is a collection of compile units from a binary,
// which can be queried using FindCompileUnit
// o find the appropriate compile unit for a PC address.
type CompileUnits struct {
	All []CompileUnit
}

// CompileUnit contains the metadata for a single compilation unit in a binary.
// For more information, see the DWARF v4 spec, section 3.1
//
// This struct is based on github.com/go-delve/delve/pkg/proc.*compileUnit:
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/bininfo.go#L436
// which is licensed under MIT.
type CompileUnit struct {
	// DWARF version of this compile unit
	Version uint8
	LowPC   uint64
	Ranges  [][2]uint64
	// debug_info entry describing this compile unit
	Entry *dwarf.Entry
}

// FindCompileUnit returns the compile unit containing address pc,
// or nil if no such compile unit was found.
//
// This implementation is based on github.com/go-delve/delve/pkg/proc.(*BinaryInfo).findCompileUnit:
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/bininfo.go#L1115
// which is licensed under MIT.
func (c *CompileUnits) FindCompileUnit(pc uint64) *CompileUnit {
	for _, cu := range c.All {
		for _, rng := range cu.Ranges {
			if pc >= rng[0] && pc < rng[1] {
				return &cu
			}
		}
	}

	return nil
}

// LoadCompileUnits scans the debug information entries (DIEs) in a binary
// for a list of compile units.
func LoadCompileUnits(dwarfData *dwarf.Data, debugInfoBytes []byte) (*CompileUnits, error) {
	offsetToVersion := util.ReadUnitVersions(debugInfoBytes)
	entryReader := dwarfData.Reader()
	compileUnits := []CompileUnit{}
	for entry, err := entryReader.Next(); entry != nil; entry, err = entryReader.Next() {
		if err != nil {
			return nil, err
		}

		if entry.Tag != dwarf.TagCompileUnit {
			entryReader.SkipChildren()
			continue
		}

		cu := CompileUnit{}
		cu.Entry = entry
		cu.Version = offsetToVersion[entry.Offset]
		cu.Ranges, _ = dwarfData.Ranges(entry)
		if len(cu.Ranges) >= 1 {
			cu.LowPC = cu.Ranges[0][0]
		}
		compileUnits = append(compileUnits, cu)
	}

	return &CompileUnits{
		All: compileUnits,
	}, nil
}
