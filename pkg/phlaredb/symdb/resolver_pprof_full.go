package symdb

import (
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type pprofFull struct {
	profile googlev1.Profile
	symbols *Symbols
	samples *schemav1.Samples
	lut     []uint32
	cur     int
}

func (r *pprofFull) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	r.profile.Sample = make([]*googlev1.Sample, samples.Len())
}

func (r *pprofFull) InsertStacktrace(_ uint32, locations []int32) {
	s := &googlev1.Sample{
		LocationId: make([]uint64, len(locations)),
		Value:      []int64{int64(r.samples.Values[r.cur])},
	}
	for i, v := range locations {
		s.LocationId[i] = uint64(v)
	}
	r.profile.Sample[r.cur] = s
	r.cur++
}

func (r *pprofFull) buildPprof() *googlev1.Profile {
	createSampleTypeStub(&r.profile)
	if r.symbols != nil {
		copyLocations(&r.profile, r.symbols, r.lut)
		copyFunctions(&r.profile, r.symbols, r.lut)
		copyMappings(&r.profile, r.symbols, r.lut)
		copyStrings(&r.profile, r.symbols, r.lut)
	}
	return &r.profile
}
