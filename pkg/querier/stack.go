package querier

import "sync"

var nodePool = sync.Pool{
	New: func() interface{} {
		return &stackNode{}
	},
}

// Stack is a stack of values. Pushing and popping values is O(1).
type Stack[T any] struct {
	top   *stackNode
	count int
}

type stackNode struct {
	val  interface{} // avoid generics so we can share the node pool
	next *stackNode
}

func NewStack[T any](initialValues ...T) *Stack[T] {
	s := &Stack[T]{}
	for _, v := range initialValues {
		s.Push(v)
	}
	return s
}

// Push adds a value to the top of the stack.
func (s *Stack[T]) Push(v T) {
	new := nodePool.Get().(*stackNode)
	new.val = v
	new.next = s.top
	s.top = new
	s.count++
}

// Pop removes and returns the top value from the stack.
func (s *Stack[T]) Pop() (result T, ok bool) {
	if s.count == 0 {
		ok = false
		return
	}
	old := s.top
	v := s.top.val
	s.top = s.top.next
	s.count--
	nodePool.Put(old)
	return v.(T), true
}

// Count returns the number of values in the stack.
func (s *Stack[T]) Count() int {
	return s.count
}

// Slice returns a slice of the values in the stack.
// The top value of the stack is at the beginning of the slice.
func (s *Stack[T]) Slice() []T {
	result := make([]T, 0, s.count)
	for n := s.top; n != nil; n = n.next {
		result = append(result, n.val.(T))
	}
	return result
}

// Release releases the stack's resources.
func (s *Stack[T]) Release() {
	for n := s.top; n != nil; n = n.next {
		nodePool.Put(n)
	}
	s.top = nil
	s.count = 0
}
