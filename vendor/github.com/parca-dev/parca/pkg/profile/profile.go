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

package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/parca-dev/parca/pkg/metastore"
)

type InstantProfileMeta struct {
	PeriodType ValueType
	SampleType ValueType
	Timestamp  int64
	Duration   int64
	Period     int64
}

type Sample struct {
	Location  []*metastore.Location
	Value     int64
	DiffValue int64
	Label     map[string][]string
	NumLabel  map[string][]int64
	NumUnit   map[string][]string
}

type ValueType struct {
	Type string
	Unit string
}

func CopyInstantProfile(p InstantProfile) *Profile {
	return &Profile{
		Meta:        p.ProfileMeta(),
		FlatSamples: p.Samples(),
	}
}

type InstantProfile interface {
	ProfileMeta() InstantProfileMeta
	Samples() map[string]*Sample
}

type StacktraceSamples struct {
	Meta    InstantProfileMeta
	Samples []*Sample
}

func StacktraceSamplesFromFlatProfile(ctx context.Context, tracer trace.Tracer, metaStore metastore.ProfileMetaStore, p InstantProfile) (*StacktraceSamples, error) {
	samples := p.Samples()

	sampleUUIDs := make([][]byte, 0, len(samples))
	for id := range samples {
		sampleUUIDs = append(sampleUUIDs, []byte(id))
	}

	sampleMap, err := metaStore.GetStacktraceByIDs(ctx, sampleUUIDs...)
	if err != nil {
		return nil, err
	}

	locationUUIDSeen := map[string]struct{}{}
	locationUUIDs := [][]byte{}
	for _, s := range sampleMap {
		for _, id := range s.GetLocationIds() {
			if _, seen := locationUUIDSeen[string(id)]; !seen {
				locationUUIDSeen[string(id)] = struct{}{}
				locationUUIDs = append(locationUUIDs, id)
			}
		}
	}

	// Get the full locations for the location UUIDs
	locationsMap, err := metastore.GetLocationsByIDs(ctx, metaStore, locationUUIDs...)
	if err != nil {
		return nil, fmt.Errorf("get locations by ids: %w", err)
	}

	ssamples := make([]*Sample, 0, len(samples))
	for k, s := range samples {
		locationIDs := sampleMap[k].GetLocationIds()
		s.Location = make([]*metastore.Location, 0, len(locationIDs))
		for _, id := range locationIDs {
			s.Location = append(s.Location, locationsMap[string(id)])
		}

		ssamples = append(ssamples, s)
	}

	return &StacktraceSamples{
		Meta:    p.ProfileMeta(),
		Samples: ssamples,
	}, nil
}

type Profile struct {
	Meta        InstantProfileMeta
	FlatSamples map[string]*Sample
}

func (fp *Profile) ProfileMeta() InstantProfileMeta {
	return fp.Meta
}

func (fp *Profile) Samples() map[string]*Sample {
	return fp.FlatSamples
}

func MetaFromPprof(p *profile.Profile, sampleIndex int) InstantProfileMeta {
	return InstantProfileMeta{
		Timestamp:  p.TimeNanos / time.Millisecond.Nanoseconds(),
		Duration:   p.DurationNanos,
		Period:     p.Period,
		PeriodType: ValueType{Type: p.PeriodType.Type, Unit: p.PeriodType.Unit},
		SampleType: ValueType{Type: p.SampleType[sampleIndex].Type, Unit: p.SampleType[sampleIndex].Unit},
	}
}

type ScaledInstantProfile struct {
	p     InstantProfile
	ratio float64
}

func NewScaledInstantProfile(p InstantProfile, ratio float64) InstantProfile {
	return &ScaledInstantProfile{
		p:     p,
		ratio: ratio,
	}
}

func (p *ScaledInstantProfile) ProfileMeta() InstantProfileMeta {
	return p.p.ProfileMeta()
}

func (p *ScaledInstantProfile) Samples() map[string]*Sample {
	samples := p.p.Samples()
	for _, s := range samples {
		s.Value = int64(p.ratio * float64(s.Value))
	}
	return samples
}

// MakeSample creates a sample from a stack trace (list of locations) and a
// value. Mostly meant for testing.
func MakeSample(value int64, locationIds []uuid.UUID) *Sample {
	s := &Sample{
		Value: value,
	}

	for _, id := range locationIds {
		s.Location = append(s.Location, &metastore.Location{ID: id})
	}

	return s
}
