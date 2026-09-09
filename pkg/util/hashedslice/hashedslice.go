// Package hashedslice provides a content-addressed, append-only slice:
// adding a value equal to one added before returns the existing index
// instead of appending, so a value's index is stable for the life of the
// slice.
package hashedslice

import "slices"

// New returns an empty Slice that uses equal to confirm matches when two
// values share a hash.
func New[A any](equal func(A, A) bool) *Slice[A] {
	return &Slice[A]{
		m:     make(map[uint64]int),
		equal: equal,
	}
}

// Slice is a content-addressed, append-only slice. Callers supply each
// value's hash; collisions are resolved by probing successive hash slots
// and confirming with the equality function, so hash quality affects
// performance, not correctness. Not safe for concurrent use.
type Slice[A any] struct {
	m     map[uint64]int
	equal func(A, A) bool

	// Values holds the distinct values in insertion order; indices returned
	// by Add point into it. Hashes is parallel to Values and records each
	// value's original hash. Both are exported for read access and must not
	// be modified.
	Values []A
	Hashes []uint64
}

// Len returns the number of distinct values added.
func (h *Slice[A]) Len() int {
	return len(h.Values)
}

// Grow ensures capacity for size additional values.
func (h *Slice[A]) Grow(size int) {
	h.Values = slices.Grow(h.Values, size)
	h.Hashes = slices.Grow(h.Hashes, size)
}

// Add returns the index of v, appending it if no equal value was added
// before. hash must be deterministic for v: equal values must supply equal
// hashes.
func (h *Slice[A]) Add(hash uint64, v A) int32 {
	for probeHash := hash; ; probeHash++ {
		idx, found := h.m[probeHash]
		if !found {
			idx = len(h.Values)
			h.m[probeHash] = idx
			h.Values = append(h.Values, v)
			h.Hashes = append(h.Hashes, hash) // store original hash, not probe offset
			return int32(idx)
		}
		if h.equal(h.Values[idx], v) {
			return int32(idx)
		}
		// hash collision: probe next slot
	}
}
