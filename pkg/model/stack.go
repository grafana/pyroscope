package model

import "sync"

var stackIntPool = sync.Pool{
	New: func() interface{} {
		return NewStack[int64]()
	},
}

var stackNodePool = sync.Pool{
	New: func() interface{} {
		return NewStack[stackNode]()
	},
}

type stackNode struct {
	xOffset int
	level   int
	node    *node
}

// Stack is a stack of values. Pushing and popping values is O(1).
type Stack[T any] struct {
	values []T
}

func NewStack[T any]() *Stack[T] {
	return &Stack[T]{}
}

// Push adds a value to the top of the stack.
func (s *Stack[T]) Push(v T) {
	s.values = append(s.values, v)
}

// Pop removes and returns the top value from the stack.
func (s *Stack[T]) Pop() (result T, ok bool) {
	if len(s.values) == 0 {
		ok = false
		return
	}
	top := s.values[len(s.values)-1]
	s.values = s.values[:len(s.values)-1]
	return top, true
}

// Slice returns a slice of the values in the stack.
// The top value of the stack is at the beginning of the slice.
func (s *Stack[T]) Slice() []T {
	result := make([]T, 0, len(s.values))
	for i := len(s.values) - 1; i >= 0; i-- {
		result = append(result, s.values[i])
	}
	return result
}

// Reset releases the stack's resources.
func (s *Stack[T]) Reset() {
	s.values = s.values[:0]
}
