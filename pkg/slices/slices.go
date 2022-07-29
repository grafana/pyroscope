package slices

// RemoveInPlace removes all elements from a slice that match the given predicate.
// Does not allocate a new slice.
func RemoveInPlace[T any](collection []T, predicate func(T, int) bool) []T {
	i := 0
	for j, x := range collection {
		if !predicate(x, j) {
			collection[i] = x
			i++
		}
	}
	return collection[:i]
}
