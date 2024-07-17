package symdb

import (
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// StacktraceIDRange represents a range of stack trace
// identifiers, sharing the same parent pointer tree.
type StacktraceIDRange struct {
	// Stack trace identifiers that belong to the range.
	// Identifiers are relative to the range Offset().
	IDs   []uint32
	chunk uint32 // Chunk index.
	m     uint32 // Max nodes per chunk.
	// Parent pointer tree, the stack traces refer to.
	// A stack trace identifier is the index of the node
	// in the tree. Optional.
	ParentPointerTree
	// Samples matching the stack trace range. Optional.
	schemav1.Samples
	// TODO(kolesnikovae): use SampleAppender instead of Samples.
	//  This will allow to avoid copying the samples.
}

// SetNodeValues sets the values of the provided Samples to the matching
// parent pointer tree nodes.
func (r *StacktraceIDRange) SetNodeValues(dst []Node) {
	for i := 0; i < len(r.IDs); i++ {
		x := r.StacktraceIDs[i]
		v := int64(r.Values[i])
		if x > 0 && v > 0 {
			dst[x].Value = v
		}
	}
}

// Offset returns the lowest identifier of the range.
// Identifiers are relative to the range offset.
func (r *StacktraceIDRange) Offset() uint32 { return r.m * r.chunk }

// SplitStacktraces splits the range of stack trace IDs by limit n into
// sub-ranges matching to the corresponding chunks and shifts the values
// accordingly. Note that the input s is modified in place.
//
// stack trace ID 0 is reserved and is not expected at the input.
// stack trace ID % max_nodes == 0 is not expected as well.
func SplitStacktraces(s []uint32, n uint32) []*StacktraceIDRange {
	if s[len(s)-1] < n || n == 0 {
		// Fast path, just one chunk: the highest stack trace ID
		// is less than the chunk size, or the size is not limited.
		// It's expected that in most cases we'll end up here.
		return []*StacktraceIDRange{{m: n, IDs: s}}
	}

	var (
		loi int
		lov = (s[0] / n) * n // Lowest possible value for the current chunk.
		hiv = lov + n        // Highest possible value for the current chunk.
		p   uint32           // Previous value (to derive chunk index).
		// 16 chunks should be more than enough in most cases.
		cs = make([]*StacktraceIDRange, 0, 16)
	)

	for i, v := range s {
		if v < hiv {
			// The stack belongs to the current chunk.
			s[i] -= lov
			p = v
			continue
		}
		lov = (v / n) * n
		hiv = lov + n
		s[i] -= lov
		cs = append(cs, &StacktraceIDRange{
			chunk: p / n,
			IDs:   s[loi:i],
			m:     n,
		})
		loi = i
		p = v
	}

	if t := s[loi:]; len(t) > 0 {
		cs = append(cs, &StacktraceIDRange{
			chunk: p / n,
			IDs:   t,
			m:     n,
		})
	}

	return cs
}
