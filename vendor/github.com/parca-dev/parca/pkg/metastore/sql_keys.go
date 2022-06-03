// Copyright 2021 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metastore

import (
	"strconv"
	"strings"

	"github.com/google/uuid"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

type MappingKey struct {
	Size, Offset  uint64
	BuildIDOrFile string
}

func MakeSQLMappingKey(m *pb.Mapping) MappingKey {
	// Normalize addresses to handle address space randomization.
	// Round up to next 4K boundary to avoid minor discrepancies.
	const mapsizeRounding = 0x1000

	size := m.Limit - m.Start
	size = size + mapsizeRounding - 1
	size = size - (size % mapsizeRounding)
	key := MappingKey{
		Size:   size,
		Offset: m.Offset,
	}

	switch {
	case m.BuildId != "":
		// BuildID has precedence over file as we can rely on it being more
		// unique.
		key.BuildIDOrFile = m.BuildId
	case m.File != "":
		key.BuildIDOrFile = m.File
	default:
		// A mapping containing neither build ID nor file name is a fake mapping. A
		// key with empty buildIDOrFile is used for fake mappings so that they are
		// treated as the same mapping during merging.
	}
	return key
}

type FunctionKey struct {
	StartLine                  int64
	Name, SystemName, Filename string
}

func MakeSQLFunctionKey(f *pb.Function) FunctionKey {
	return FunctionKey{
		f.StartLine,
		f.Name,
		f.SystemName,
		f.Filename,
	}
}

type LocationKey struct {
	Address   uint64
	MappingID uuid.UUID
	Lines     string
	IsFolded  bool
}

func MakeSQLLocationKey(l *Location) LocationKey {
	key := LocationKey{
		Address:  l.Address,
		IsFolded: l.IsFolded,
	}
	if l.Mapping != nil {
		mUUID, err := uuid.FromBytes(l.Mapping.Id)
		if err != nil {
			panic(err)
		}
		key.MappingID = mUUID
	}

	// If the address is 0, then the functions attached to the
	// location are not from a native binary, but instead from a dynamic
	// runtime/language eg. ruby or python. In those cases we have no better
	// uniqueness factor than the actual functions, and since there is no
	// address there is no potential for asynchronously symbolizing.
	if key.Address == 0 {
		lines := make([]string, len(l.Lines)*2)
		for i, line := range l.Lines {
			if line.Function != nil {
				fID, err := uuid.FromBytes(line.Function.Id)
				if err != nil {
					panic(err)
				}
				lines[i*2] = fID.String()
			}
			lines[i*2+1] = strconv.FormatInt(line.Line, 16)
		}
		key.Lines = strings.Join(lines, "|")
	}
	return key
}
