// Copyright 2022 The Parca Authors
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

package parcacol

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/prometheus/prometheus/model/labels"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
	"github.com/parca-dev/parca/pkg/metastore"
)

type Table interface {
	Schema() *dynparquet.Schema
	InsertBuffer(context.Context, *dynparquet.Buffer) (tx uint64, err error)
}

type Ingester struct {
	logger    log.Logger
	table     Table
	metaStore metastore.ProfileMetaStore // Swap with local interface
}

func NewIngester(logger log.Logger, metaStore metastore.ProfileMetaStore, table Table) *Ingester {
	return &Ingester{logger: logger, metaStore: metaStore, table: table}
}

var ErrMissingNameLabel = errors.New("missing __name__ label")

func (ing Ingester) Ingest(ctx context.Context, inLs labels.Labels, p *profile.Profile, normalized bool) error {
	samples, err := ing.ConvertPProf(ctx, inLs, p, normalized)
	if err != nil {
		return err
	}

	for _, s := range samples {
		if err := ing.IngestSamples(ctx, s); err != nil {
			return err
		}
	}

	return nil
}

func (ing Ingester) ConvertPProf(ctx context.Context, inLs labels.Labels, p *profile.Profile, normalized bool) ([]Samples, error) {
	// We need to extract the name from the labels into a separate column.
	// The labels are the same excluding the __name__.
	var name string
	ls := make(labels.Labels, 0, len(inLs))
	for _, l := range inLs {
		if l.Name == labels.MetricName {
			name = l.Value
		} else {
			ls = append(ls, l)
		}
	}
	if name == "" {
		return nil, ErrMissingNameLabel
	}
	sort.Sort(ls)

	samples := make([]Samples, 0, len(p.SampleType))
	for i := range p.SampleType {
		pn := &profileNormalizer{
			logger:    ing.logger,
			metaStore: ing.metaStore,

			samples:       make(map[string]*Sample, len(p.Sample)),
			locationsByID: make(map[uint64]*metastore.Location, len(p.Location)),
			functionsByID: make(map[uint64]*pb.Function, len(p.Function)),
			mappingsByID:  make(map[uint64]mapInfo, len(p.Mapping)),
		}

		if p.TimeNanos == 0 {
			return nil, errors.New("timestamp must not be zero")
		}
		if len(p.Sample) == 0 {
			// Ignore profiles with no samples
			continue
		}

		// meta data that all samples share
		meta := sampleMeta{
			Name:       name,
			Labels:     ls,
			SampleType: p.SampleType[i].Type,
			SampleUnit: p.SampleType[i].Unit,
			PeriodType: p.PeriodType.Type,
			PeriodUnit: p.PeriodType.Unit,
			Duration:   p.DurationNanos,
			Period:     p.Period,
			Timestamp:  p.TimeNanos / time.Millisecond.Nanoseconds(),
		}

		// All samples for this sample type
		typeSamples := make(Samples, 0, len(p.Sample))
		for _, s := range p.Sample {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
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

				sample, _, err := pn.mapSample(ctx, s, meta, i, normalized)
				if err != nil {
					return nil, err
				}

				typeSamples = append(typeSamples, sample)
			}
		}
		samples = append(samples, typeSamples)
	}

	return samples, nil
}

func (ing Ingester) IngestSamples(ctx context.Context, samples Samples) error {
	buffer, err := samples.ToBuffer(Schema())
	if err != nil {
		return fmt.Errorf("failed to convert samples to buffer: %w", err)
	}

	buffer.Sort()

	// This is necessary because sorting a buffer makes concurrent reading not
	// safe as the internal pages are cyclically sorted at read time. Cloning
	// executes the cyclic sort once and makes the resulting buffer safe for
	// concurrent reading as it no longer has to perform the cyclic sorting at
	// read time. This should probably be improved in the parquet library.
	buffer, err = buffer.Clone()
	if err != nil {
		return err
	}

	_, err = ing.table.InsertBuffer(ctx, buffer)
	if err != nil {
		return fmt.Errorf("failed to insert buffer: %w", err)
	}

	return nil
}

func isZeroSample(s *profile.Sample) bool {
	for _, v := range s.Value {
		if v != 0 {
			return false
		}
	}
	return true
}

type profileNormalizer struct {
	logger    log.Logger
	metaStore metastore.ProfileMetaStore

	samples map[string]*Sample
	// Memoization tables within a profile.
	locationsByID map[uint64]*metastore.Location
	functionsByID map[uint64]*pb.Function
	mappingsByID  map[uint64]mapInfo
}

type mapInfo struct {
	m      *pb.Mapping
	offset int64
}

type sampleMeta struct {
	Name       string
	Labels     labels.Labels
	SampleType string
	SampleUnit string
	PeriodType string
	PeriodUnit string
	Period     int64
	Duration   int64
	Timestamp  int64
}

func (pn *profileNormalizer) mapSample(ctx context.Context, s *profile.Sample, meta sampleMeta, index int, normalized bool) (*Sample, bool, error) {
	sn := &SampleNormalizer{
		Location: make([]*metastore.Location, len(s.Location)),
		Label:    make(map[string]string, len(s.Label)),
		NumLabel: make(map[string]int64, len(s.NumLabel)),
		NumUnit:  make(map[string]string, len(s.NumLabel)),
	}

	var err error
	for i, l := range s.Location {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
			sn.Location[i], err = pn.mapLocation(ctx, l, normalized)
			if err != nil {
				return nil, false, err
			}
		}
	}
	for k, v := range s.Label {
		if len(v) == 1 {
			sn.Label[k] = v[0]
		}
	}
	for k, v := range s.NumLabel {
		if len(v) == 1 {
			sn.NumLabel[k] = v[0]
		}
		u := s.NumUnit[k]
		if len(u) == 1 {
			sn.NumUnit[k] = u[0]
		}
	}

	// Check memoization table. Must be done on the remapped location to
	// account for the remapped mapping. Add current values to the
	// existing sample.
	k := MakeStacktraceKey(sn)

	stacktraceUUID, err := pn.metaStore.GetStacktraceByKey(ctx, k)
	if err != nil && err != metastore.ErrStacktraceNotFound {
		return nil, false, err
	}

	if stacktraceUUID == uuid.Nil {
		pbs := &pb.Sample{}
		pbs.LocationIds = make([][]byte, 0, len(sn.Location))
		for _, l := range sn.Location {
			pbs.LocationIds = append(pbs.LocationIds, l.ID[:])
		}

		pbs.Labels = make(map[string]*pb.SampleLabel, len(sn.Label))
		for l, strings := range sn.Label {
			pbs.Labels[l] = &pb.SampleLabel{Labels: []string{strings}}
		}

		pbs.NumLabels = make(map[string]*pb.SampleNumLabel, len(sn.NumLabel))
		for l, int64s := range sn.NumLabel {
			pbs.NumLabels[l] = &pb.SampleNumLabel{NumLabels: []int64{int64s}}
		}

		pbs.NumUnits = make(map[string]*pb.SampleNumUnit, len(sn.NumUnit))
		for l, strings := range sn.NumUnit {
			pbs.NumUnits[l] = &pb.SampleNumUnit{Units: []string{strings}}
		}

		stacktraceUUID, err = pn.metaStore.CreateStacktrace(ctx, k, pbs)
		if err != nil {
			return nil, false, err
		}
	}

	sa, found := pn.samples[string(stacktraceUUID[:])]
	if found {
		sa.Value += s.Value[index]
		return sa, false, nil
	}

	pn.samples[string(stacktraceUUID[:])] = &Sample{
		Name:       meta.Name,
		Labels:     meta.Labels,
		Duration:   meta.Duration,
		Period:     meta.Period,
		PeriodType: meta.PeriodType,
		PeriodUnit: meta.PeriodUnit,
		SampleType: meta.SampleType,
		SampleUnit: meta.SampleUnit,
		Timestamp:  meta.Timestamp,

		Stacktrace:     stacktraceUUID[:],
		PprofLabels:    sn.Label,
		PprofNumLabels: sn.NumLabel,
		Value:          s.Value[index],
	}

	return pn.samples[string(stacktraceUUID[:])], true, nil
}

type SampleNormalizer struct {
	Location []*metastore.Location
	Label    map[string]string
	NumLabel map[string]int64
	NumUnit  map[string]string
}

func (pn *profileNormalizer) mapLocation(ctx context.Context, src *profile.Location, normalized bool) (*metastore.Location, error) {
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

func (pn *profileNormalizer) mapMapping(ctx context.Context, src *profile.Mapping) (mapInfo, error) {
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

func (pn *profileNormalizer) mapLine(ctx context.Context, src profile.Line) (metastore.LocationLine, error) {
	f, err := pn.mapFunction(ctx, src.Function)
	if err != nil {
		return metastore.LocationLine{}, err
	}

	return metastore.LocationLine{
		Function: f,
		Line:     src.Line,
	}, nil
}

func (pn *profileNormalizer) mapFunction(ctx context.Context, src *profile.Function) (*pb.Function, error) {
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

type StacktraceKey []byte

// MakeStacktraceKey generates StacktraceKey to be used as a key for maps.
func MakeStacktraceKey(sample *SampleNormalizer) StacktraceKey {
	numLocations := len(sample.Location)
	if numLocations == 0 {
		return []byte{}
	}

	locationLength := (16 * numLocations) + (numLocations - 1)

	labelsLength := 0
	labelNames := make([]string, 0, len(sample.Label))
	for k, v := range sample.Label {
		labelNames = append(labelNames, k)
		labelsLength += len(k) + len(v) + 4 + 1 // 4 for quotes, 1 for colon
	}
	sort.Strings(labelNames)

	numLabelsLength := 0
	numLabelsNames := make([]string, 0, len(sample.NumLabel))
	for k := range sample.NumLabel {
		numLabelsNames = append(numLabelsNames, k)
		numLabelsLength += len(k) + 2                 // key + 2 quotes
		numLabelsLength += 2                          // colon + curly brace
		numLabelsLength += len(sample.NumUnit[k]) + 2 // unit + 2 quotes
		numLabelsLength += 1                          // colon
		numLabelsLength += 8                          // 64bit
		numLabelsLength += 1                          // curly brace
	}
	sort.Strings(numLabelsNames)

	length := locationLength + labelsLength + numLabelsLength
	key := make([]byte, 0, length)

	for i, l := range sample.Location {
		key = append(key, l.ID[:]...)
		if i != len(sample.Location)-1 {
			key = append(key, '|')
		}
	}

	for i := 0; i < len(sample.Label); i++ {
		l := labelNames[i]
		v := sample.Label[l]
		key = append(key, '"')
		key = append(key, l...)
		key = append(key, '"')
		key = append(key, ':')
		key = append(key, '"')
		key = append(key, v...)
		key = append(key, '"')
	}

	for i := 0; i < len(sample.NumLabel); i++ {
		l := numLabelsNames[i]
		v := sample.NumLabel[l]
		key = append(key, '"')
		key = append(key, l...)
		key = append(key, '"')
		key = append(key, ':')
		key = append(key, '{')
		key = append(key, '"')
		key = append(key, sample.NumUnit[l]...)
		key = append(key, '"')
		key = append(key, ':')
		for shift := 56; shift >= 0; shift -= 8 {
			key = append(key, byte(v>>shift))
		}
		key = append(key, '}')
	}

	return key
}
