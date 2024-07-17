package symdb

import (
	"strings"
	"unsafe"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type pprofGoPGO struct {
	profile googlev1.Profile
	symbols *Symbols
	samples *schemav1.Samples
	pgo     *typesv1.GoPGO
	stacks  map[string]*googlev1.Sample
	lut     []uint32
	cur     int
}

func (r *pprofGoPGO) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	// It's expected that after trimmed, most of
	// the stack traces will be deduplicated.
	r.stacks = make(map[string]*googlev1.Sample, samples.Len()/50)
}

func (r *pprofGoPGO) InsertStacktrace(_ uint32, locations []int32) {
	if len(locations) == 0 {
		r.cur++
		return
	}
	if n := int(r.pgo.KeepLocations); n > 0 && len(locations) > n {
		locations = locations[:n]
	}
	// Trimming implies that many samples will have the same
	// stack trace (the expected value for keepLocs is 5).
	// Therefore, the map is read-intensive: we speculatively
	// reuse capacity of the locations slice as the key, and
	// only copy it, if the insertion into the map is required.
	k := int32sliceString(locations)
	sample, ok := r.stacks[k]
	if !ok {
		sample = &googlev1.Sample{
			LocationId: make([]uint64, len(locations)),
			Value:      []int64{0},
		}
		for i, v := range locations {
			sample.LocationId[i] = uint64(v)
		}
		// Do not retain the input slice.
		r.stacks[strings.Clone(k)] = sample
	}
	sample.Value[0] += int64(r.samples.Values[r.cur])
	r.cur++
}

func int32sliceString(u []int32) string {
	return unsafe.String((*byte)(unsafe.Pointer(&u[0])), len(u)*4)
}

func (r *pprofGoPGO) buildPprof() *googlev1.Profile {
	createSampleTypeStub(&r.profile)
	r.appendSamples()
	if r.symbols != nil {
		copyLocations(&r.profile, r.symbols, r.lut)
		copyFunctions(&r.profile, r.symbols, r.lut)
		copyMappings(&r.profile, r.symbols, r.lut)
		copyStrings(&r.profile, r.symbols, r.lut)
	}
	if r.pgo.AggregateCallees {
		// Actual aggregation occurs at pprof.Merge call after
		// the profile is built. All unreferenced objects are
		// to be removed from the profile, and samples with
		// matching stack traces are to be merged.
		r.clearCalleeLineNumber()
	}
	return &r.profile
}

func (r *pprofGoPGO) appendSamples() {
	r.profile.Sample = make([]*googlev1.Sample, len(r.stacks))
	var i int
	for _, s := range r.stacks {
		r.profile.Sample[i] = s
		i++
	}
}

func (r *pprofGoPGO) clearCalleeLineNumber() {
	for _, s := range r.profile.Sample {
		r.profile.Location[s.LocationId[0]-1].Line[0].Line = 0
	}
}
