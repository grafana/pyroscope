package querier

// minHeap is a custom min-heap data structure that stores integers.
type minHeap []int64

// Len returns the number of elements in the min-heap.
func (h minHeap) Len() int { return len(h) }

// Less returns true if the element at index i is less than the element at index j.
// This method is used by the container/heap package to maintain the min-heap property.
func (h minHeap) Less(i, j int) bool { return h[i] < h[j] }

// Swap exchanges the elements at index i and index j.
// This method is used by the container/heap package to reorganize the min-heap during its operations.
func (h minHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push adds an element (x) to the min-heap.
// This method is used by the container/heap package to grow the min-heap.
func (h *minHeap) Push(x interface{}) {
	*h = append(*h, x.(int64))
}

// Pop removes and returns the smallest element (minimum) from the min-heap.
// This method is used by the container/heap package to shrink the min-heap.
func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
