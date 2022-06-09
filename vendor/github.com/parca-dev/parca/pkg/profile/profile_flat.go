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

package profile

import (
	"context"

	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/metastore"
)

// ProfilesFromPprof extracts a Profile from each sample index included in the pprof profile.
func ProfilesFromPprof(ctx context.Context, l log.Logger, s metastore.ProfileMetaStore, p *profile.Profile, normalized bool) ([]*Profile, error) {
	ps := make([]*Profile, 0, len(p.SampleType))

	for i := range p.SampleType {
		p, err := FromPprof(ctx, l, s, p, i, normalized)
		if err != nil {
			return nil, err
		}
		if p != nil {
			ps = append(ps, p)
		}
	}
	return ps, nil
}

func FromPprof(ctx context.Context, logger log.Logger, metaStore metastore.ProfileMetaStore, p *profile.Profile, sampleIndex int, normalized bool) (*Profile, error) {
	pfn := &profileFlatNormalizer{
		logger:    logger,
		metaStore: metaStore,

		samples:       make(map[string]*Sample, len(p.Sample)),
		locationsByID: make(map[uint64]*metastore.Location, len(p.Location)),
		functionsByID: make(map[uint64]*pb.Function, len(p.Function)),
		mappingsByID:  make(map[uint64]mapInfo, len(p.Mapping)),
	}

	for _, s := range p.Sample {
		if isZeroSample(s) {
			continue
		}

		// TODO: This is semantically incorrect, it is valid to have no
		// locations in pprof. This needs to be fixed once we remove the
		// stacktrace UUIDs since location IDs are going to be saved directly
		// in the columnstore.
		if len(s.Location) == 0 {
			continue
		}

		_, _, err := pfn.mapSample(ctx, s, sampleIndex, normalized)
		if err != nil {
			return nil, err
		}
	}

	if len(pfn.samples) == 0 {
		return nil, nil
	}

	return &Profile{
		Meta:        MetaFromPprof(p, sampleIndex),
		FlatSamples: pfn.samples,
	}, nil
}

func isZeroSample(s *profile.Sample) bool {
	for _, v := range s.Value {
		if v != 0 {
			return false
		}
	}
	return true
}

type profileFlatNormalizer struct {
	logger    log.Logger
	metaStore metastore.ProfileMetaStore

	samples map[string]*Sample
	// Memoization tables within a profile.
	locationsByID map[uint64]*metastore.Location
	functionsByID map[uint64]*pb.Function
	mappingsByID  map[uint64]mapInfo
}

func (pn *profileFlatNormalizer) mapSample(ctx context.Context, src *profile.Sample, sampleIndex int, normalized bool) (*Sample, bool, error) {
	var err error

	s := &Sample{
		Location: make([]*metastore.Location, len(src.Location)),
		Label:    make(map[string][]string, len(src.Label)),
		NumLabel: make(map[string][]int64, len(src.NumLabel)),
		NumUnit:  make(map[string][]string, len(src.NumLabel)),
	}
	for i, l := range src.Location {
		s.Location[i], err = pn.mapLocation(ctx, l, normalized)
		if err != nil {
			return nil, false, err
		}
	}
	for k, v := range src.Label {
		vv := make([]string, len(v))
		copy(vv, v)
		s.Label[k] = vv
	}
	for k, v := range src.NumLabel {
		u := src.NumUnit[k]
		vv := make([]int64, len(v))
		uu := make([]string, len(u))
		copy(vv, v)
		copy(uu, u)
		s.NumLabel[k] = vv
		s.NumUnit[k] = uu
	}
	// Check memoization table. Must be done on the remapped location to
	// account for the remapped mapping. Add current values to the
	// existing sample.
	k := MakeStacktraceKey(s)

	stacktraceUUID, err := pn.metaStore.GetStacktraceByKey(ctx, k)
	if err != nil && err != metastore.ErrStacktraceNotFound {
		return nil, false, err
	}
	if stacktraceUUID == uuid.Nil {
		pbs := &pb.Sample{}
		pbs.LocationIds = make([][]byte, 0, len(s.Location))
		for _, l := range s.Location {
			pbs.LocationIds = append(pbs.LocationIds, l.ID[:])
		}

		pbs.Labels = make(map[string]*pb.SampleLabel, len(s.Label))
		for l, strings := range s.Label {
			pbs.Labels[l] = &pb.SampleLabel{Labels: strings}
		}

		pbs.NumLabels = make(map[string]*pb.SampleNumLabel, len(s.NumLabel))
		for l, int64s := range s.NumLabel {
			pbs.NumLabels[l] = &pb.SampleNumLabel{NumLabels: int64s}
		}

		pbs.NumUnits = make(map[string]*pb.SampleNumUnit, len(s.NumUnit))
		for l, strings := range s.NumUnit {
			pbs.NumUnits[l] = &pb.SampleNumUnit{Units: strings}
		}

		stacktraceUUID, err = pn.metaStore.CreateStacktrace(ctx, k, pbs)
		if err != nil {
			return nil, false, err
		}
	}

	sa, found := pn.samples[string(stacktraceUUID[:])]
	if found {
		sa.Value += src.Value[sampleIndex]
		return sa, false, nil
	}

	s.Value += src.Value[sampleIndex]
	pn.samples[string(stacktraceUUID[:])] = s
	return s, true, nil
}

func (pn *profileFlatNormalizer) mapLocation(ctx context.Context, src *profile.Location, normalized bool) (*metastore.Location, error) {
	var err error

	if src == nil {
		return nil, nil
	}

	if l, ok := pn.locationsByID[src.ID]; ok {
		return l, nil
	}

	mi, err := pn.mapMapping(ctx, src.Mapping)
	if err != nil {
		return nil, err
	}

	var addr uint64
	if !normalized {
		addr = uint64(int64(src.Address) + mi.offset)
	} else {
		addr = src.Address
	}

	l := &metastore.Location{
		Mapping:  mi.m,
		Address:  addr,
		Lines:    make([]metastore.LocationLine, len(src.Line)),
		IsFolded: src.IsFolded,
	}
	for i, ln := range src.Line {
		l.Lines[i], err = pn.mapLine(ctx, ln)
		if err != nil {
			return nil, err
		}
	}
	// Check memoization table. Must be done on the remapped location to
	// account for the remapped mapping ID.
	loc, err := metastore.GetLocationByKey(ctx, pn.metaStore, l)
	if err != nil && err != metastore.ErrLocationNotFound {
		return nil, err
	}
	if loc != nil {
		pn.locationsByID[src.ID] = loc
		return loc, nil
	}
	pn.locationsByID[src.ID] = l

	id, err := pn.metaStore.CreateLocation(ctx, l)
	if err != nil {
		return nil, err
	}

	l.ID, err = uuid.FromBytes(id)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (pn *profileFlatNormalizer) mapMapping(ctx context.Context, src *profile.Mapping) (mapInfo, error) {
	if src == nil {
		return mapInfo{}, nil
	}

	if mi, ok := pn.mappingsByID[src.ID]; ok {
		return mi, nil
	}

	// Check memoization tables.
	m, err := pn.metaStore.GetMappingByKey(ctx, &pb.Mapping{
		Start:   src.Start,
		Limit:   src.Limit,
		Offset:  src.Offset,
		File:    src.File,
		BuildId: src.BuildID,
	})
	if err != nil && err != metastore.ErrMappingNotFound {
		return mapInfo{}, err
	}
	if m != nil {
		// NOTICE: We only store a single version of a mapping.
		// Which means the m.Start actually correct for a single process.
		// For a multi-process shared library, this will always be wrong.
		// And storing the mapping for each process will be very expensive.
		// Which is why the client sending the profiling data can choose to normalize the addresses for each process.
		// In a future iteration of the wire format, the computed base address for each mapping should be included
		// to prevent this dilemma or forcing the client to be smart in one direction or the other.
		mi := mapInfo{m, int64(src.Start) - int64(m.Start)}
		pn.mappingsByID[src.ID] = mi
		return mi, nil
	}
	m = &pb.Mapping{
		Start:           src.Start,
		Limit:           src.Limit,
		Offset:          src.Offset,
		File:            src.File,
		BuildId:         src.BuildID,
		HasFunctions:    src.HasFunctions,
		HasFilenames:    src.HasFilenames,
		HasLineNumbers:  src.HasLineNumbers,
		HasInlineFrames: src.HasInlineFrames,
	}

	// Update memoization tables.
	id, err := pn.metaStore.CreateMapping(ctx, m)
	if err != nil {
		return mapInfo{}, err
	}
	m.Id = id
	mi := mapInfo{m, 0}
	pn.mappingsByID[src.ID] = mi
	return mi, nil
}

func (pn *profileFlatNormalizer) mapLine(ctx context.Context, src profile.Line) (metastore.LocationLine, error) {
	f, err := pn.mapFunction(ctx, src.Function)
	if err != nil {
		return metastore.LocationLine{}, err
	}

	return metastore.LocationLine{
		Function: f,
		Line:     src.Line,
	}, nil
}

func (pn *profileFlatNormalizer) mapFunction(ctx context.Context, src *profile.Function) (*pb.Function, error) {
	if src == nil {
		return nil, nil
	}
	if f, ok := pn.functionsByID[src.ID]; ok {
		return f, nil
	}
	f, err := pn.metaStore.GetFunctionByKey(ctx, &pb.Function{
		Name:       src.Name,
		SystemName: src.SystemName,
		Filename:   src.Filename,
		StartLine:  src.StartLine,
	})
	if err != nil && err != metastore.ErrFunctionNotFound {
		return nil, err
	}
	if f != nil {
		pn.functionsByID[src.ID] = f
		return f, nil
	}
	f = &pb.Function{
		Name:       src.Name,
		SystemName: src.SystemName,
		Filename:   src.Filename,
		StartLine:  src.StartLine,
	}

	id, err := pn.metaStore.CreateFunction(ctx, f)
	if err != nil {
		return nil, err
	}
	f.Id = id

	pn.functionsByID[src.ID] = f
	return f, nil
}
