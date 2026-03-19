package symdb

import (
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type pprofLocTree struct {
	symbols *Symbols
	samples *schemav1.Samples
	profile googlev1.Profile
	lut     []uint32
	cur     int

	maxNodes  int64
	truncated int
	// Sum of fully truncated samples.
	fullyTruncated int64

	locTree      *model.StacktraceTree
	stacktraces  []truncatedStacktraceSample
	locationsBuf []int32
	sampleMap    map[string]*googlev1.Sample
}

func (r *pprofLocTree) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	// We optimistically assume that each stacktrace has only
	// 2 unique nodes. For pathological cases it may exceed 10.
	r.locTree = model.NewStacktraceTree(samples.Len() * 2)
	r.stacktraces = make([]truncatedStacktraceSample, 0, samples.Len())
	r.sampleMap = make(map[string]*googlev1.Sample, samples.Len())
}

func (r *pprofLocTree) InsertStacktrace(_ uint32, locations []int32) {
	value := int64(r.samples.Values[r.cur])
	r.cur++
	locNodeIdx := r.locTree.Insert(locations, value)
	r.stacktraces = append(r.stacktraces, truncatedStacktraceSample{
		nodeIdx: locNodeIdx,
		value:   value,
	})
}

func (r *pprofLocTree) buildPprof() *googlev1.Profile {
	r.markNodesForTruncation()
	for _, n := range r.stacktraces {
		r.addSample(n)
	}
	r.createSamples()
	createSampleTypeStub(&r.profile)
	copyLocations(&r.profile, r.symbols, r.lut)
	copyFunctions(&r.profile, r.symbols, r.lut)
	copyMappings(&r.profile, r.symbols, r.lut)
	copyStrings(&r.profile, r.symbols, r.lut)
	if r.truncated > 0 || r.fullyTruncated > 0 {
		createLocationStub(&r.profile)
	}
	return &r.profile
}

func (r *pprofLocTree) markNodesForTruncation() {
	// We preserve more nodes than requested to preserve more
	// locations with inlined functions. The multiplier is
	// chosen empirically; it should be roughly equal to the
	// ratio of nodes in the location tree to the nodes in the
	// function tree (after truncation).
	minValue := r.locTree.MinValue(r.maxNodes * 4)
	if minValue == 0 {
		return
	}
	for i := range r.locTree.Nodes {
		if r.locTree.Nodes[i].Total < minValue {
			r.locTree.Nodes[i].Location |= truncationMark
			r.truncated++
		}
	}
}

func (r *pprofLocTree) addSample(n truncatedStacktraceSample) {
	r.locationsBuf = r.buildLocationsStack(r.locationsBuf, n.nodeIdx)
	if len(r.locationsBuf) == 0 {
		// The stack has no functions without the truncation mark.
		r.fullyTruncated += n.value
		return
	}
	if s, ok := r.sampleMap[int32sliceString(r.locationsBuf)]; ok {
		s.Value[0] += n.value
		return
	}

	locationsCopy := make([]uint64, len(r.locationsBuf))
	for i := 0; i < len(r.locationsBuf); i++ {
		locationsCopy[i] = uint64(r.locationsBuf[i])
	}

	s := &googlev1.Sample{LocationId: locationsCopy, Value: []int64{n.value}}
	r.profile.Sample = append(r.profile.Sample, s)

	k := make([]int32, len(r.locationsBuf))
	copy(k, r.locationsBuf)
	r.sampleMap[int32sliceString(k)] = s
}

func (r *pprofLocTree) buildLocationsStack(dst []int32, idx int32) []int32 {
	dst = dst[:0]
	for i := idx; i > 0; i = r.locTree.Nodes[i].Parent {
		if r.locTree.Nodes[i].Location&truncationMark == 0 {
			dst = append(dst, r.locTree.Nodes[i].Location&^truncationMark)
		} else if len(dst) == 0 {
			dst = append(dst, truncationMark)
		}
	}
	if len(dst) == 1 && dst[0] == truncationMark {
		return dst[:0]
	}
	return dst
}

func (r *pprofLocTree) createSamples() {
	samples := len(r.sampleMap)
	r.profile.Sample = make([]*googlev1.Sample, 0, samples+1)
	for _, s := range r.sampleMap {
		r.profile.Sample = append(r.profile.Sample, s)
	}
	if r.fullyTruncated > 0 {
		r.profile.Sample = append(r.profile.Sample, &googlev1.Sample{
			LocationId: []uint64{truncationMark},
			Value:      []int64{r.fullyTruncated},
		})
	}
}
