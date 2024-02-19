package pprof

import (
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// FixGoProfile removes type parameters from Go function names.
//
// In go 1.21 and above, function names include type parameters,
// however, due to the bug, a function name can include any
// of the type instances regardless of the call site. Thus, e.g.,
// x[T1].foo and x[T2].foo can't be distinguished in a profile.
// This leads to incorrect profiles and incorrect flame graphs,
// and hugely increases cardinality of stack traces.
//
// FixGoProfile will change x[T1].foo and x[T2].foo to x[...].foo
// and normalize the profile, if type parameters are present in
// the profile. Otherwise, the profile returned unchanged.
func FixGoProfile(p *profilev1.Profile) *profilev1.Profile {
	var n int
	for i, s := range p.StringTable {
		c := DropGoTypeParameters(p.StringTable[i])
		if c != s {
			p.StringTable[i] = c
			n++
		}
	}
	if n == 0 {
		return p
	}
	// Merging with self effectively normalizes the profile:
	// it removed duplicates, establishes order of labels,
	// and allocates monotonic identifiers.
	var m ProfileMerge
	// We safely ignore the error as the only case when it can
	// happen is when merged profiles are of different types.
	_ = m.Merge(p)
	return m.Profile()
}
