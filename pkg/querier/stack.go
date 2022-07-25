package querier

import "sync"

var stackIntPool = sync.Pool{
	New: func() interface{} {
		return NewStack[int]()
	},
}

var stackNodePool = sync.Pool{
	New: func() interface{} {
		return NewStack[*node]()
	},
}

// Stack is a stack of values. Pushing and popping values is O(1).
type Stack[T any] struct {
	values []T
	next   []int
	top    int
	count  int
}

type stackNode struct {
	val  interface{} // avoid generics so we can share the node pool
	next *stackNode
}

func NewStack[T any](initialValues ...T) *Stack[T] {
	s := &Stack[T]{
		values: make([]T, len(initialValues)),
		next:   make([]int, len(initialValues)),
		top:    -1,
		count:  len(initialValues),
	}
	if s.count == 0 {
		return s
	}
	for pos, v := range initialValues {
		s.values[pos] = v
		s.next[pos] = pos + 1
	}
	s.top = 0
	s.next[s.count-1] = -1
	return s
}

// Push adds a value to the top of the stack.
func (s *Stack[T]) Push(v T) {
	s.values = append(s.values, v)
	s.next = append(s.next, s.top)
	s.top = len(s.values) - 1
	s.count++
}

// Pop removes and returns the top value from the stack.
func (s *Stack[T]) Pop() (result T, ok bool) {
	if s.top == -1 {
		ok = false
		return
	}
	old := s.top
	v := s.values[old]
	s.top = s.next[old]
	s.count--
	return v, true
}

// Count returns the number of values in the stack.
func (s *Stack[T]) Count() int {
	return s.count
}

// Slice returns a slice of the values in the stack.
// The top value of the stack is at the beginning of the slice.
func (s *Stack[T]) Slice() []T {
	result := make([]T, 0, s.count)
	for n := s.top; n != -1; n = s.next[n] {
		result = append(result, s.values[n])
	}
	return result
}

// Release releases the stack's resources.
func (s *Stack[T]) Release() {
	s.values = s.values[:0]
	s.next = s.next[:0]
	s.top = -1
	s.count = 0
}
