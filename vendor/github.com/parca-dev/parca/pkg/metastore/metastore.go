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
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

var (
	ErrStacktraceNotFound = errors.New("stacktrace not found")
	ErrLocationNotFound   = errors.New("location not found")
	ErrMappingNotFound    = errors.New("mapping not found")
	ErrFunctionNotFound   = errors.New("function not found")
)

type ProfileMetaStore interface {
	StacktraceStore
	LocationStore
	LocationLineStore
	FunctionStore
	MappingStore
	Close() error
	Ping() error
}

type StacktraceStore interface {
	GetStacktraceByKey(ctx context.Context, key []byte) (uuid.UUID, error)
	GetStacktraceByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Sample, error)
	CreateStacktrace(ctx context.Context, key []byte, sample *pb.Sample) (uuid.UUID, error)
}

type LocationStore interface {
	GetLocations(ctx context.Context) ([]*pb.Location, [][]byte, error)
	GetLocationByKey(ctx context.Context, key *Location) (*pb.Location, error)
	GetLocationsByIDs(ctx context.Context, id ...[]byte) (map[string]*pb.Location, [][]byte, error)
	CreateLocation(ctx context.Context, l *Location) ([]byte, error)
	Symbolize(ctx context.Context, location *Location) error
	GetSymbolizableLocations(ctx context.Context) ([]*pb.Location, [][]byte, error)
}

type LocationLineStore interface {
	CreateLocationLines(ctx context.Context, locID []byte, lines []LocationLine) error
	GetLinesByLocationIDs(ctx context.Context, id ...[]byte) (map[string][]*pb.Line, [][]byte, error)
}

type Location struct {
	ID       uuid.UUID
	Address  uint64
	Mapping  *pb.Mapping
	Lines    []LocationLine
	IsFolded bool
}

type LocationLine struct {
	Line     int64
	Function *pb.Function
}

type FunctionStore interface {
	GetFunctionByKey(ctx context.Context, key *pb.Function) (*pb.Function, error)
	CreateFunction(ctx context.Context, f *pb.Function) ([]byte, error)
	GetFunctionsByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Function, error)
	GetFunctions(ctx context.Context) ([]*pb.Function, error)
}

type MappingStore interface {
	GetMappingByKey(ctx context.Context, key *pb.Mapping) (*pb.Mapping, error)
	CreateMapping(ctx context.Context, m *pb.Mapping) ([]byte, error)
	GetMappingsByIDs(ctx context.Context, ids ...[]byte) (map[string]*pb.Mapping, error)
}

// UnsymbolizableMapping returns true if a mapping points to a binary for which
// locations can't be symbolized in principle, at least now. Examples are
// "[vdso]", [vsyscall]" and some others, see the code.
func UnsymbolizableMapping(m *pb.Mapping) bool {
	name := filepath.Base(m.File)
	return strings.HasPrefix(name, "[") || strings.HasPrefix(name, "linux-vdso") || strings.HasPrefix(m.File, "/dev/dri/")
}

func GetLocationByKey(ctx context.Context, s ProfileMetaStore, key *Location) (*Location, error) {
	res := Location{}

	l, err := s.GetLocationByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	res.ID, err = uuid.FromBytes(l.Id)
	if err != nil {
		return nil, err
	}

	res.Address = l.Address
	res.IsFolded = l.IsFolded

	if l.MappingId != nil {
		mappings, err := s.GetMappingsByIDs(ctx, l.MappingId)
		if err != nil {
			return nil, fmt.Errorf("get mapping by ID: %w", err)
		}
		res.Mapping = mappings[string(l.MappingId)]
	}

	linesByLocation, functionIDs, err := s.GetLinesByLocationIDs(ctx, l.Id)
	if err != nil {
		return nil, fmt.Errorf("get lines by location ID: %w", err)
	}

	functions, err := s.GetFunctionsByIDs(ctx, functionIDs...)
	if err != nil {
		return nil, fmt.Errorf("get functions by IDs: %w", err)
	}

	for _, line := range linesByLocation[string(l.Id)] {
		res.Lines = append(res.Lines, LocationLine{
			Line:     line.Line,
			Function: functions[string(line.FunctionId)],
		})
	}

	return &res, nil
}

func GetLocationsByIDs(ctx context.Context, s ProfileMetaStore, ids ...[]byte) (
	map[string]*Location,
	error,
) {
	locs, mappingIDs, err := s.GetLocationsByIDs(ctx, ids...)
	if err != nil {
		return nil, fmt.Errorf("get locations by IDs: %w", err)
	}

	return getLocationsFromSerializedLocations(ctx, s, ids, locs, mappingIDs)
}

// Only used in tests so not as important to be efficient.
func GetLocations(ctx context.Context, s ProfileMetaStore) ([]*Location, error) {
	lArr, mappingIDs, err := s.GetLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get serialized locations: %w", err)
	}

	l := map[string]*pb.Location{}
	locIDs := [][]byte{}
	for _, loc := range lArr {
		l[string(loc.Id)] = loc
		locIDs = append(locIDs, loc.Id)
	}

	locs, err := getLocationsFromSerializedLocations(ctx, s, locIDs, l, mappingIDs)
	if err != nil {
		return nil, fmt.Errorf("get locations: %w", err)
	}

	res := make([]*Location, 0, len(locs))
	for _, loc := range locs {
		res = append(res, loc)
	}

	return res, nil
}

func getLocationsFromSerializedLocations(
	ctx context.Context,
	s ProfileMetaStore,
	ids [][]byte,
	locs map[string]*pb.Location,
	mappingIDs [][]byte,
) (
	map[string]*Location,
	error,
) {
	mappings, err := s.GetMappingsByIDs(ctx, mappingIDs...)
	if err != nil {
		return nil, fmt.Errorf("get mappings by IDs: %w", err)
	}

	linesByLocation, functionIDs, err := s.GetLinesByLocationIDs(ctx, ids...)
	if err != nil {
		return nil, fmt.Errorf("get lines by location IDs: %w", err)
	}

	functions, err := s.GetFunctionsByIDs(ctx, functionIDs...)
	if err != nil {
		return nil, fmt.Errorf("get functions by ids: %w", err)
	}

	res := make(map[string]*Location, len(locs))
	for locationID, loc := range locs {
		locID, err := uuid.FromBytes([]byte(locationID))
		if err != nil {
			return nil, err
		}

		location := &Location{
			ID:       locID,
			Address:  loc.Address,
			IsFolded: loc.IsFolded,
			Mapping:  mappings[string(loc.MappingId)],
		}

		locationLines := linesByLocation[locationID]
		if len(locationLines) > 0 {
			lines := make([]LocationLine, 0, len(locationLines))
			for _, line := range locationLines {
				function, found := functions[string(line.FunctionId)]
				if found {
					lines = append(lines, LocationLine{
						Line:     line.Line,
						Function: function,
					})
				}
			}
			location.Lines = lines
		}
		res[locationID] = location
	}

	return res, nil
}

func GetSymbolizableLocations(ctx context.Context, s ProfileMetaStore) (
	[]*Location,
	error,
) {
	locs, mappingIDs, err := s.GetSymbolizableLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get symbolizable locations: %w", err)
	}

	mappings, err := s.GetMappingsByIDs(ctx, mappingIDs...)
	if err != nil {
		return nil, fmt.Errorf("get mappings by IDs: %w", err)
	}

	res := make([]*Location, 0, len(locs))
	for _, loc := range locs {
		id, err := uuid.FromBytes(loc.Id)
		if err != nil {
			return nil, err
		}

		res = append(res, &Location{
			ID:       id,
			Address:  loc.Address,
			IsFolded: loc.IsFolded,
			Mapping:  mappings[string(loc.MappingId)],
		})
	}

	return res, nil
}
