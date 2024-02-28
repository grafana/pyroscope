package pprof

import (
	"bytes"
	"reflect"
	"sort"
	"unsafe"

	"golang.org/x/exp/slices"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

const (
	minGroupSize = 2

	tokens    = 8
	tokenLen  = 16
	suffixLen = tokens + tokenLen

	tokenBytesLen  = tokenLen * 8
	suffixBytesLen = suffixLen * 8
)

// MayHaveGoHeapTruncatedStacktraces reports whether there are
// any chances that the profile may have truncated stack traces.
func MayHaveGoHeapTruncatedStacktraces(p *profilev1.Profile) bool {
	if !hasGoHeapSampleTypes(p) {
		return false
	}
	// Some truncated stacks have depth less than the depth limit (32).
	const minDepth = 28
	for _, s := range p.Sample {
		if len(s.LocationId) >= minDepth {
			return true
		}
	}
	return false
}

func hasGoHeapSampleTypes(p *profilev1.Profile) bool {
	for _, st := range p.SampleType {
		switch p.StringTable[st.Type] {
		case
			"alloc_objects",
			"alloc_space",
			"inuse_objects",
			"inuse_space":
			return true
		}
	}
	return false
}

// RepairGoHeapTruncatedStacktraces repairs truncated stack traces
// in Go heap profiles.
//
// Go heap profile has a depth limit of 32 frames, which often
// renders profiles unreadable, and also increases cardinality
// of stack traces.
//
// The function guesses truncated frames based on neighbors and
// repairs stack traces if there are high chances that this
// part is present in the profile. The heuristic is as follows:
//
// For each stack trace S taller than 24 frames: if there is another
// stack trace R taller than 24 frames that overlaps with the given
// one by at least 16 frames in a row from the top, and has frames
// above its root, stack S considered truncated, and the missing part
// is copied from R.
func RepairGoHeapTruncatedStacktraces(p *profilev1.Profile) {
	// Group stack traces by bottom (closest to the root) locations.
	// Typically, there are very few groups (a hundred or two).
	samples, groups := split(p)
	// Each group's suffix is then tokenized: each part is shifted by one
	// location from the previous one (like n-grams).
	// Tokens are written into the token=>group map, Where the value is the
	// index of the group with the token found at the furthest position from
	// the root (across all groups).
	m := make(map[string]group, len(groups)/2)
	for i := 0; i < len(groups); i++ {
		g := groups[i]
		n := len(groups)
		if i+1 < len(groups) {
			n = groups[i+1]
		}
		if s := n - g; s < minGroupSize {
			continue
		}
		// We take suffix of the first sample in the group.
		s := suffix(samples[g].LocationId)
		// Tokenize the suffix: token position is relative to the stack
		// trace root: 0 means that the token is the closest to the root.
		//    TODO: unroll?
		//    0 : 64 : 192 // No need.
		//    1 : 56 : 184
		//    2 : 48 : 176
		//    3 : 40 : 168
		//    4 : 32 : 160
		//    5 : 24 : 152
		//    6 : 16 : 144
		//    7 :  8 : 136
		//    8 :  0 : 128
		//
		// We skip the top/right-most token, as it is not needed,
		// because there can be no more complete stack trace.
		for j := uint32(1); j <= tokens; j++ {
			hi := suffixBytesLen - j*tokens
			lo := hi - tokenBytesLen
			// By taking a string representation of the slice,
			// we eliminate the need to hash the token explicitly:
			// Go map will do it this way or another.
			k := unsafeString(s[lo:hi])
			// Current candidate: the group where the token is
			// located at the furthest position from the root.
			c, ok := m[k]
			if !ok || j > c.off {
				// This group has more complete stack traces:
				m[k] = group{
					// gid 0 is reserved as a sentinel value.
					gid: uint32(i + 1),
					off: j,
				}
			}
		}
	}

	// Now we handle chaining. Consider the following stacks:
	//  1   2   3   4
	//  a   b  [d] (f)
	//  b   c  [e] (g)
	//  c  [d] (f)  h
	//  d  [e] (g)  i
	//
	// We can't associate 3-rd stack with the 1-st one because their tokens
	// do not overlap (given the token size is 2). However, we can associate
	// it transitively through the 2nd stack.
	//
	// Dependencies:
	//  - group i depends on d[i].
	//  - d[i] depends on d[d[i].gid-1].
	d := make([]group, len(groups))
	for i := 0; i < len(groups); i++ {
		g := groups[i]
		t := topToken(samples[g].LocationId)
		k := unsafeString(t)
		c, ok := m[k]
		if !ok || c.gid-1 == uint32(i) {
			// The current group has the most complete stack trace.
			continue
		}
		d[i] = c
	}

	// Then, for each group, we test, if there is another group with a more
	// complete suffix, overlapping with the given one by at least one token.
	// If such stack trace exists, all stack traces of the group are appended
	// with the missing part.
	for i := 0; i < len(groups); i++ {
		g := groups[i]
		c := d[i]
		var off uint32
		var j int
		for c.gid > 0 && c.off > 0 {
			off += c.off
			n := d[c.gid-1]
			if n.gid == 0 || c.off == 0 {
				// Stop early to preserve c.
				break
			}
			c = n
			j++
			if j == tokenLen {
				// Profiles with deeply recursive stack traces are ignored.
				return
			}
		}
		if off == 0 {
			// The current group has the most complete stack trace.
			continue
		}
		// The reference stack trace.
		appx := samples[groups[c.gid-1]].LocationId
		// It's possible that the reference stack trace does not
		// include the part we're looking for. In this case, we
		// simply ignore the group. Although it's possible to infer
		// this piece from other stacks, this is left for further
		// improvements.
		if int(off) >= len(appx) {
			continue
		}
		appx = appx[uint32(len(appx))-off:]
		// Now we append the missing part to all stack traces of the group.
		n := len(groups)
		if i+1 < len(groups) {
			n = groups[i+1]
		}
		for j := g; j < n; j++ {
			// Locations typically already have some extra capacity,
			// therefore no major allocations are expected here.
			samples[j].LocationId = append(samples[j].LocationId, appx...)
		}
	}
}

type group struct {
	gid uint32
	off uint32
}

// suffix returns the last suffixLen locations
// of the given stack trace represented as bytes.
// The return slice is always suffixBytesLen long.
// function panics if s is shorter than suffixLen.
func suffix(s []uint64) []byte {
	return locBytes(s[len(s)-suffixLen:])
}

// topToken returns the last tokenLen locations
// of the given stack trace represented as bytes.
// The return slice is always tokenBytesLen long.
// function panics if s is shorter than tokenLen.
func topToken(s []uint64) []byte {
	return locBytes(s[len(s)-tokenLen:])
}

func locBytes(s []uint64) []byte {
	size := len(s) * 8
	h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	h.Len = size
	h.Cap = size
	return *(*[]byte)(unsafe.Pointer(h))
}

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// split into groups of samples by stack trace suffixes.
// Return slice contains indices of the first sample
// of each group in the collection of selected samples.
func split(p *profilev1.Profile) ([]*profilev1.Sample, []int) {
	slices.SortFunc(p.Sample, func(a, b *profilev1.Sample) int {
		if len(a.LocationId) < suffixLen {
			return -1
		}
		if len(b.LocationId) < suffixLen {
			return 1
		}
		return bytes.Compare(
			suffix(a.LocationId),
			suffix(b.LocationId),
		)
	})
	o := sort.Search(len(p.Sample), func(i int) bool {
		return len(p.Sample[i].LocationId) >= suffixLen
	})
	if o == len(p.Sample) {
		return nil, nil
	}
	samples := p.Sample[o:]
	const avgGroupSize = 16 // Estimate.
	groups := make([]int, 0, len(samples)/avgGroupSize)
	var prev []byte
	for i := 0; i < len(samples); i++ {
		cur := suffix(samples[i].LocationId)
		if !bytes.Equal(cur, prev) {
			groups = append(groups, i)
			prev = cur
		}
	}
	return samples, groups
}
