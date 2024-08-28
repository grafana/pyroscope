package slices

import (
	"slices"
)

// RemoveInPlace removes all elements from a slice that match the given predicate.
// Does not allocate a new slice.
func RemoveInPlace[T any](collection []T, predicate func(T, int) bool) []T {
	i := 0
	for j, x := range collection {
		if !predicate(x, j) {
			collection[j], collection[i] = collection[i], collection[j]
			i++
		}
	}
	return collection[:i]
}

func Reverse[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func Clear[S ~[]E, E any](s S) {
	var zero E
	for i := range s {
		s[i] = zero
	}
}

func GrowLen[S ~[]E, E any](s S, n int) S {
	s = s[:0]
	return slices.Grow(s, n)[:n]
}
