package pprof

import (
	"strings"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// FixGoProfile fixes known issues with profiles collected with
// the standard Go profiler.
//
// Note that the function presumes that p is a Go profile and does
// not verify this. It is expected that the function is called
// very early in the profile processing chain and normalized after,
// regardless of the function outcome.
func FixGoProfile(p *profilev1.Profile) *profilev1.Profile {
	p = DropGoTypeParameters(p)
	// Now that the profile is normalized, we can try to repair
	// truncated stack traces, if any. Note that repaired stacks
	// are not deduplicated, so the caller need to normalize the
	if PotentialTruncatedGoStacktraces(p) {
		RepairGoTruncatedStacktraces(p)
	}
	return p
}

// DropGoTypeParameters removes of type parameters from Go function names.
//
// In go 1.21 and above, function names include type parameters, however,
// due to a bug, a function name could include any of the type instances
// regardless of the call site. Thus, e.g., x[T1].foo and x[T2].foo can't
// be distinguished in a profile. This leads to incorrect profiles and
// incorrect flame graphs, and hugely increases cardinality of stack traces.
//
// The function renames x[T1].foo and x[T2].foo to x[...].foo and normalizes
// the profile, if type parameters are present in the profile. Otherwise, the
// profile returns unchanged.
//
// See https://github.com/golang/go/issues/64528.
func DropGoTypeParameters(p *profilev1.Profile) *profilev1.Profile {
	var n int
	for i, s := range p.StringTable {
		c := dropGoTypeParameters(s)
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
	_ = m.Merge(p, true)
	return m.Profile()
}

const goShapePrefix = "[go.shape."

func dropGoTypeParameters(input string) string {
	if !strings.Contains(input, goShapePrefix) {
		return input
	}

	var result strings.Builder
	i := 0

	for i < len(input) {
		// Find next type parameter section
		start := strings.Index(input[i:], goShapePrefix)
		if start < 0 {
			// No more type parameters, write the rest
			result.WriteString(input[i:])
			break
		}

		// Write everything before this type parameter
		result.WriteString(input[i : i+start])

		// Find matching closing bracket by tracking depth
		depth := 0
		j := i + start
		for j < len(input) {
			if input[j] == '[' {
				depth++
			} else if input[j] == ']' {
				depth--
				if depth == 0 {
					// Found matching closing bracket, skip to after it
					i = j + 1
					break
				}
			}
			j++
		}

		if depth != 0 {
			// No matching bracket found, keep rest as-is
			result.WriteString(input[i:])
			break
		}
	}

	return result.String()
}
