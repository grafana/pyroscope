package tree

import (
	"testing"
)

//todo test empty tree

type mockStackBuilder struct {
	ss [][]byte

	stackID2Stack map[uint64]string
	stackID2Val   map[uint64]uint64
}

func (s *mockStackBuilder) Push(frame []byte) {
	s.ss = append(s.ss, frame)
}

func (s *mockStackBuilder) Pop() {
	s.ss = s.ss[0 : len(s.ss)-1]
}

func (s *mockStackBuilder) Build() (stackID uint64) {
	res := ""
	for _, bs := range s.ss {
		if len(res) != 0 {
			res += ";"
		}
		res += string(bs)
	}
	id := uint64(len(s.stackID2Stack))
	s.stackID2Stack[id] = res
	return id
}

func (s *mockStackBuilder) Reset() {
	s.ss = s.ss[:0]
}

func (s *mockStackBuilder) expectValue(t *testing.T, stackId, expected uint64) {
	if s.stackID2Val[stackId] != expected {
		t.Fatalf("expected at %d %d got %d", stackId, expected, s.stackID2Val[stackId])
	}
}
func (s *mockStackBuilder) expectStack(t *testing.T, stackId uint64, expected string) {
	if s.stackID2Stack[stackId] != expected {
		t.Fatalf("expected at %d %s got %s", stackId, expected, s.stackID2Stack[stackId])
	}
}

func TestIterateWithStackBuilder(t *testing.T) {
	sb := &mockStackBuilder{stackID2Stack: make(map[uint64]string), stackID2Val: make(map[uint64]uint64)}
	tree := New()
	tree.Insert([]byte("a;b"), uint64(1))
	tree.Insert([]byte("a;c"), uint64(2))
	tree.Insert([]byte("a;d;e"), uint64(3))
	tree.Insert([]byte("a;d;f"), uint64(4))

	tree.IterateWithStackBuilder(sb, func(stackID uint64, v uint64) {
		sb.stackID2Val[stackID] = v
	})
	sb.expectValue(t, 0, 1)
	sb.expectValue(t, 1, 2)
	sb.expectValue(t, 2, 3)
	sb.expectValue(t, 3, 4)
	sb.expectStack(t, 0, "a;b")
	sb.expectStack(t, 1, "a;c")
	sb.expectStack(t, 2, "a;d;e")
	sb.expectStack(t, 3, "a;d;f")
}

func TestIterateWithStackBuilderEmpty(t *testing.T) {
	tree := New()
	sb := &mockStackBuilder{stackID2Stack: make(map[uint64]string), stackID2Val: make(map[uint64]uint64)}
	tree.IterateWithStackBuilder(sb, func(stackID uint64, v uint64) {
		t.Fatal()
	})
}
