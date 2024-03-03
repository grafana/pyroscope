// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package dwarfutils

import (
	"debug/dwarf"
	"fmt"
)

// GetChildLeafEntries returns a list of the debugging information entries (DIEs)
// that are direct children of the entry at the given offset.
func GetChildLeafEntries(entryReader *dwarf.Reader, offset dwarf.Offset, tag dwarf.Tag) ([]*dwarf.Entry, error) {
	entryReader.Seek(offset)

	// Consume the first element from the reader to move it past the parent entry
	parentEntry, err := entryReader.Next()
	if err != nil {
		return nil, fmt.Errorf("couldn't consume initial parent entry from entry reader")
	}
	if !parentEntry.Children {
		return nil, nil
	}

	// entryReader should now be set up to iterate on the direct children of the parent entry
	childLeafEntries := []*dwarf.Entry{}
	for childEntry, err := entryReader.Next(); childEntry != nil; childEntry, err = entryReader.Next() {
		if err != nil {
			return nil, fmt.Errorf("encountered error when iterating parent entry's children: %w", err)
		}

		// If this entry is null, then we reached the end of the tree
		if IsEntryNull(childEntry) {
			break
		}

		if childEntry.Tag == tag {
			childLeafEntries = append(childLeafEntries, childEntry)
		}

		// If this entry has children, then skip them
		if childEntry.Children {
			entryReader.SkipChildren()
			continue
		}
	}

	return childLeafEntries, nil
}

// IsEntryNull determines whether a *dwarf.Entry value is a "null" DIE,
// which are used to denote the end of sub-trees.
// Note that this is not the same as a nil *dwarf.Entry value;
// a "null" DIE has a specific meaning in DWARF.
// For more information, see the DWARF v4 spec, section 2.3
func IsEntryNull(entry *dwarf.Entry) bool {
	// Every field is 0 in an empty entry
	return !entry.Children &&
		len(entry.Field) == 0 &&
		entry.Offset == 0 &&
		entry.Tag == dwarf.Tag(0)
}
