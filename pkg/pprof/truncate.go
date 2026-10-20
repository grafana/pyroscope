package pprof

import (
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Post-merge node-limit enforcement for merged pprof profiles.
//
// The node budget in the v2 read path is applied per symbol partition: the
// resolver limits each resolved partition to maxNodes function tree nodes.
// Once the partitions are merged, the budget no longer holds — the merged
// profile can contain up to maxNodes nodes per partition, which is what
// makes SelectMergeProfile responses unbounded (#5841).
//
// Truncate re-applies the budget to the merged profile, mirroring the
// per-partition semantics (phlaredb/symdb pprofTree): the function tree is
// rebuilt over all merged samples, the maxNodes-th largest node total
// becomes the cut-off threshold, and every branch with a smaller total is
// replaced with a single synthetic "other" frame. Stacks that fall below
// the threshold entirely are aggregated into a single synthetic sample.
//
// Retained locations are kept verbatim, therefore address, mapping, and
// inlining information of the retained frames is preserved, and the
// profile remains valid for symbolization. Objects that become
// unreferenced by the enforcement are removed and identifiers are made
// dense — otherwise the budget would not reduce the marshaled size.

const (
	truncationMark    = 1 << 30
	truncatedNodeName = "other"
)

// Truncate limits the number of function tree nodes of the profile to
// maxNodes. The profile is modified in place and returned; if maxNodes is
// non-positive, or the profile does not exceed the budget, or the profile
// references objects in a way that prevents the function tree from being
// built, the profile is returned unchanged.
func Truncate(p *profilev1.Profile, maxNodes int64) *profilev1.Profile {
	if p == nil || maxNodes < 1 || len(p.Sample) == 0 {
		return p
	}
	// Identifier lookups. Merged profiles have dense identifiers, but
	// the tree is keyed by the function position, not the identifier,
	// so no correctness relies on the invariant.
	locByID := make(map[uint64]*profilev1.Location, len(p.Location))
	for _, l := range p.Location {
		locByID[l.Id] = l
	}
	fnPos := make(map[uint64]int32, len(p.Function))
	for i, f := range p.Function {
		fnPos[f.Id] = int32(i) + 1 // Positions are 1-based.
	}

	// Rebuild the function tree over the merged samples. Function stacks
	// are traversed exactly as the per-partition implementation does:
	// sample locations in order (leaf first), and each location's lines
	// in order. The first sample value is used as the node weight,
	// consistent with the per-partition implementation.
	type stackRef struct {
		node int32
		fns  []int32
	}
	tree := model.NewStacktraceTree(len(p.Sample) * 2)
	refs := make([]stackRef, len(p.Sample))
	var funcs []int32
	for i, s := range p.Sample {
		if len(s.Value) == 0 {
			continue // Zero-valued samples do not affect the tree.
		}
		funcs = funcs[:0]
		for _, id := range s.LocationId {
			l, ok := locByID[id]
			if !ok {
				return p // Malformed profile; leave it unchanged.
			}
			for _, ln := range l.Line {
				if _, ok := fnPos[ln.FunctionId]; !ok {
					return p
				}
				funcs = append(funcs, fnPos[ln.FunctionId])
			}
		}
		if len(funcs) == 0 {
			// The stack has no functions (e.g. unsymbolized
			// locations) and cannot be represented in the
			// function tree; the sample is kept verbatim.
			continue
		}
		refs[i].node = tree.Insert(funcs, s.Value[0])
		refs[i].fns = append(refs[i].fns, funcs...)
	}

	minValue := tree.MinValue(maxNodes)
	if minValue == 0 {
		return p // The merged tree already fits the budget.
	}
	for i := range tree.Nodes {
		if tree.Nodes[i].Total < minValue {
			tree.Nodes[i].Location |= truncationMark
		}
	}

	// Rebuild the samples with the marks applied.
	var (
		samples        []*profilev1.Sample
		sampleIndex    = make(map[string]int, len(p.Sample))
		fullyTruncated []int64
		path           []int32
	)
	keep := func(s *profilev1.Sample, stack []uint64) {
		// Enforcement can collapse distinct stacks onto the same
		// stack; identical stacks (and labels) are aggregated.
		// Untouched stacks are remembered too: a cut stack may
		// become identical to a stack that was kept in full.
		key := sampleKey(stack, s.Label)
		if i, ok := sampleIndex[key]; ok {
			for j := range samples[i].Value {
				if j < len(s.Value) {
					samples[i].Value[j] += s.Value[j]
				}
			}
			return
		}
		sampleIndex[key] = len(samples)
		samples = append(samples, &profilev1.Sample{
			LocationId: stack,
			Value:      append([]int64(nil), s.Value...),
			// Labels are cloned, not aliased: compact rewrites the
			// label string indices in place, and the samples that
			// the profile carried before the truncation must not
			// observe it. Cloning also detaches the new slice from
			// any entries hidden past the length of the original
			// one (see GroupSamplesWithoutLabelsByKey): a cap walk
			// on the rebuilt samples cannot surface stale string
			// indices. Callers must restore such hidden labels
			// before truncating; they are not part of the visible
			// profile and are not preserved.
			Label: cloneLabels(s.Label),
		})
	}
	for i, s := range p.Sample {
		if refs[i].node == 0 {
			// The sample does not participate in the function
			// tree; it is preserved verbatim.
			keep(s, append([]uint64(nil), s.LocationId...))
			continue
		}
		// Walk the function path from the leaf to the root and
		// determine the first frame that survives (pprofTree.
		// buildFunctionsStack).
		path = path[:0]
		offset := -1
		for j := refs[i].node; j > 0; j = tree.Nodes[j].Parent {
			n := tree.Nodes[j]
			if offset < 0 && n.Location&truncationMark == 0 {
				offset = len(path)
			}
			path = append(path, n.Location&^truncationMark)
		}
		if offset < 0 {
			// The entire stack is below the threshold.
			for j, v := range s.Value {
				for j >= len(fullyTruncated) {
					fullyTruncated = append(fullyTruncated, 0)
				}
				fullyTruncated[j] += v
			}
			continue
		}
		stack := s.LocationId
		if offset > 0 {
			// Leaf-side frames are below the threshold: replace
			// them with the stub location, keeping the root-side
			// suffix (pprofTree.truncateLocations). Note that
			// when l reaches 0 the stack is kept as is, which
			// mirrors the per-partition behavior on the same
			// boundary.
			f := len(refs[i].fns)
			l := len(stack)
			for ; l > 0 && f >= offset; l-- {
				f -= len(locByID[stack[l-1]].Line)
			}
			if l > 0 {
				cut := make([]uint64, 0, 1+len(stack)-l)
				cut = append(cut, truncationMark)
				cut = append(cut, stack[l:]...)
				stack = cut
			}
		}
		keep(s, append([]uint64(nil), stack...))
	}
	var truncatedSomething bool
	for _, v := range fullyTruncated {
		if v != 0 {
			truncatedSomething = true
			break
		}
	}
	if truncatedSomething {
		samples = append(samples, &profilev1.Sample{
			LocationId: []uint64{truncationMark},
			Value:      fullyTruncated,
		})
	}

	p.Sample = samples
	compact(p)
	return p
}

// compact rewrites the profile keeping only the objects reachable from
// the samples and the profile headers, assigning dense identifiers, and
// creating the synthetic "other" location for cut frames. The stub frame
// mirrors pprofTree.createLocationStub.
func compact(p *profilev1.Profile) {
	// Position lookups for the pass below.
	locPos := make(map[uint64]int, len(p.Location))
	for i, l := range p.Location {
		locPos[l.Id] = i
	}
	fnPosIdx := make(map[uint64]int, len(p.Function))
	for i, f := range p.Function {
		fnPosIdx[f.Id] = i
	}
	mappingByID := make(map[uint64]*profilev1.Mapping, len(p.Mapping))
	for _, m := range p.Mapping {
		mappingByID[m.Id] = m
	}

	// Collect the referenced locations. The stub placeholder id is
	// recorded as a regular reference.
	stubUsed := false
	locRemap := make(map[uint64]uint64, len(p.Location))
	locations := make([]*profilev1.Location, 0, len(p.Location))
	markLocation := func(id uint64) {
		if _, ok := locRemap[id]; ok {
			return
		}
		locRemap[id] = uint64(len(locations)) + 1
		if id == truncationMark {
			stubUsed = true
			return
		}
		locations = append(locations, p.Location[locPos[id]])
	}
	for _, s := range p.Sample {
		for _, id := range s.LocationId {
			markLocation(id)
		}
	}

	// Functions referenced by the retained locations.
	fnRemap := make(map[uint64]uint64, len(p.Function))
	functions := make([]*profilev1.Function, 0, len(p.Function))
	for _, l := range locations {
		for _, ln := range l.Line {
			if _, ok := fnRemap[ln.FunctionId]; !ok {
				fnRemap[ln.FunctionId] = uint64(len(functions)) + 1
				functions = append(functions, p.Function[fnPosIdx[ln.FunctionId]])
			}
		}
	}

	// Mappings referenced by the retained locations.
	mapRemap := make(map[uint64]uint64, len(p.Mapping))
	mappings := make([]*profilev1.Mapping, 0, len(p.Mapping))
	for _, l := range locations {
		if l.MappingId == 0 {
			continue
		}
		if _, ok := mapRemap[l.MappingId]; !ok {
			m, ok := mappingByID[l.MappingId]
			if !ok {
				// Dangling mapping reference: drop the reference.
				l.MappingId = 0
				continue
			}
			mapRemap[l.MappingId] = uint64(len(mappings)) + 1
			mappings = append(mappings, m)
		}
	}

	// Strings referenced by the profile headers and the retained
	// objects. Index 0 is always the empty string.
	strRemap := make(map[int64]int64, len(p.StringTable))
	strings := make([]string, 1, len(p.StringTable))
	var markString func(id int64)
	markString = func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := strRemap[id]; !ok {
			if int(id) < len(p.StringTable) && int(id) > 0 {
				strRemap[id] = int64(len(strings))
				strings = append(strings, p.StringTable[id])
			}
		}
	}
	markValueType := func(v *profilev1.ValueType) {
		if v == nil {
			return
		}
		markString(v.Type)
		markString(v.Unit)
	}
	for _, st := range p.SampleType {
		markValueType(st)
	}
	markValueType(p.PeriodType)
	markString(p.DropFrames)
	markString(p.KeepFrames)
	markString(p.DefaultSampleType)
	for _, f := range functions {
		markString(f.Name)
		markString(f.SystemName)
		markString(f.Filename)
	}
	for _, m := range mappings {
		markString(m.Filename)
		markString(m.BuildId)
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			markString(l.Key)
			markString(l.Str)
			markString(l.NumUnit)
		}
	}
	for _, c := range p.Comment {
		markString(c)
	}

	// The synthetic "other" function and location, when the enforcement
	// actually cut frames. The stub location is appended last, so that it
	// does not claim the identifier of a retained location.
	var (
		stubLocationID uint64
		stubLocation   *profilev1.Location
		stubFunction   *profilev1.Function
	)
	if stubUsed {
		stubString := lookupString(p, truncatedNodeName)
		var stubStringNew int64
		if stubString > 0 {
			if v, ok := strRemap[stubString]; ok {
				stubStringNew = v
			} else {
				stubStringNew = int64(len(strings))
				strRemap[stubString] = stubStringNew
				strings = append(strings, truncatedNodeName)
			}
		} else {
			stubStringNew = int64(len(strings))
			strings = append(strings, truncatedNodeName)
		}
		stubFunctionID := uint64(len(functions)) + 1
		stubFunction = &profilev1.Function{
			Id:         stubFunctionID,
			Name:       stubStringNew,
			SystemName: stubStringNew,
		}
		functions = append(functions, stubFunction)
		// A location must reference a mapping; if the profile has
		// no mappings, a stub one is created (as in symdb).
		if len(mappings) == 0 {
			mappings = append(mappings, &profilev1.Mapping{Id: 1})
		}
		stubLocationID = uint64(len(locations)) + 1
		stubLocation = &profilev1.Location{
			Id:        stubLocationID,
			Line:      []*profilev1.Line{{FunctionId: stubFunctionID}},
			MappingId: 1,
		}
		locations = append(locations, stubLocation)
	}

	// Rewrite the identifiers to the new dense scheme. The stub
	// placeholder is resolved to the stub location identifier.
	for _, s := range p.Sample {
		for i, id := range s.LocationId {
			if id == truncationMark {
				s.LocationId[i] = stubLocationID
				continue
			}
			if nid, ok := locRemap[id]; ok {
				s.LocationId[i] = nid
			}
		}
	}
	for i, l := range locations {
		l.Id = uint64(i) + 1
		if l == stubLocation {
			// The stub location references the final stub
			// function and mapping identifiers already; it has no
			// entries in the remap tables.
			continue
		}
		for _, ln := range l.Line {
			ln.FunctionId = fnRemap[ln.FunctionId]
		}
		if l.MappingId != 0 {
			l.MappingId = mapRemap[l.MappingId]
		}
	}
	for i, f := range functions {
		f.Id = uint64(i) + 1
		if f == stubFunction {
			// The stub function references its final string index
			// already; it has no entry in the string remap table.
			continue
		}
		f.Name = strRemap[f.Name]
		f.SystemName = strRemap[f.SystemName]
		f.Filename = strRemap[f.Filename]
	}
	for i, m := range mappings {
		m.Id = uint64(i) + 1
		m.Filename = strRemap[m.Filename]
		m.BuildId = strRemap[m.BuildId]
	}
	rewriteString := func(id int64) int64 {
		if id == 0 {
			return 0
		}
		return strRemap[id]
	}
	for _, st := range p.SampleType {
		st.Type = rewriteString(st.Type)
		st.Unit = rewriteString(st.Unit)
	}
	if p.PeriodType != nil {
		p.PeriodType.Type = rewriteString(p.PeriodType.Type)
		p.PeriodType.Unit = rewriteString(p.PeriodType.Unit)
	}
	p.DropFrames = rewriteString(p.DropFrames)
	p.KeepFrames = rewriteString(p.KeepFrames)
	p.DefaultSampleType = rewriteString(p.DefaultSampleType)
	for i, c := range p.Comment {
		p.Comment[i] = rewriteString(c)
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			l.Key = rewriteString(l.Key)
			l.Str = rewriteString(l.Str)
			l.NumUnit = rewriteString(l.NumUnit)
		}
	}

	p.Location = locations
	p.Function = functions
	p.Mapping = mappings
	p.StringTable = strings
}

// lookupString returns the index of the first occurrence of the string in
// the profile string table, or 0 if it is not present (the empty string
// occupies index 0).
func lookupString(p *profilev1.Profile, s string) int64 {
	for i, v := range p.StringTable {
		if v == s {
			return int64(i)
		}
	}
	return 0
}

// cloneLabels returns a deep copy of the label slice: the slice and the
// label objects it references are owned exclusively by the result.
func cloneLabels(labels []*profilev1.Label) []*profilev1.Label {
	if len(labels) == 0 {
		return nil
	}
	c := make([]*profilev1.Label, len(labels))
	for i, l := range labels {
		if l == nil {
			continue
		}
		c[i] = &profilev1.Label{
			Key:     l.Key,
			Str:     l.Str,
			Num:     l.Num,
			NumUnit: l.NumUnit,
		}
	}
	return c
}

// sampleKey identifies a sample by its stack and labels. It follows the
// same aggregation semantics as ProfileMerge: samples that share the
// stack and the labels are aggregated.
func sampleKey(stack []uint64, labels []*profilev1.Label) string {
	hl := hashLocations(stack)
	ht := hashLabels(labels)
	return string([]byte{
		byte(hl), byte(hl >> 8), byte(hl >> 16), byte(hl >> 24),
		byte(hl >> 32), byte(hl >> 40), byte(hl >> 48), byte(hl >> 56),
		byte(ht), byte(ht >> 8), byte(ht >> 16), byte(ht >> 24),
		byte(ht >> 32), byte(ht >> 40), byte(ht >> 48), byte(ht >> 56),
	})
}
