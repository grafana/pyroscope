package loser_test

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/util/loser"
)

type List struct {
	list []uint64
	cur  uint64

	err error

	closed int
}

func NewList(list ...uint64) *List {
	return &List{list: list}
}

func (it *List) At() uint64 {
	return it.cur
}

func (it *List) Err() error { return it.err }

func (it *List) Next() bool {
	if it.err != nil {
		return false
	}
	if len(it.list) > 0 {
		it.cur = it.list[0]
		it.list = it.list[1:]
		return true
	}
	it.cur = 0
	return false
}

func (it *List) Close() { it.closed += 1 }

func (it *List) Seek(val uint64) bool {
	if it.err != nil {
		return false
	}
	for it.cur < val && len(it.list) > 0 {
		it.cur = it.list[0]
		it.list = it.list[1:]
	}
	return len(it.list) > 0
}

func checkIterablesEqual[E any, S1 loser.Sequence, S2 loser.Sequence](t *testing.T, a S1, b S2, at1 func(S1) E, at2 func(S2) E, less func(E, E) bool) {
	t.Helper()
	count := 0
	for a.Next() {
		count++
		if !b.Next() {
			t.Fatalf("b ended before a after %d elements", count)
		}
		if less(at1(a), at2(b)) || less(at2(b), at1(a)) {
			t.Fatalf("position %d: %v != %v", count, at1(a), at2(b))
		}
	}
	if b.Next() {
		t.Fatalf("a ended before b after %d elements", count)
	}
}

var testCases = []struct {
	name string
	args []*List
	want *List
}{
	{
		name: "empty input",
		want: NewList(),
	},
	{
		name: "one list",
		args: []*List{NewList(1, 2, 3, 4)},
		want: NewList(1, 2, 3, 4),
	},
	{
		name: "two lists",
		args: []*List{NewList(3, 4, 5), NewList(1, 2)},
		want: NewList(1, 2, 3, 4, 5),
	},
	{
		name: "two lists, first empty",
		args: []*List{NewList(), NewList(1, 2)},
		want: NewList(1, 2),
	},
	{
		name: "two lists, second empty",
		args: []*List{NewList(1, 2), NewList()},
		want: NewList(1, 2),
	},
	{
		name: "two lists b",
		args: []*List{NewList(1, 2), NewList(3, 4, 5)},
		want: NewList(1, 2, 3, 4, 5),
	},
	{
		name: "two lists c",
		args: []*List{NewList(1, 3), NewList(2, 4, 5)},
		want: NewList(1, 2, 3, 4, 5),
	},
	{
		name: "three lists",
		args: []*List{NewList(1, 3), NewList(2, 4), NewList(5)},
		want: NewList(1, 2, 3, 4, 5),
	},
}

func TestMerge(t *testing.T) {
	at := func(s *List) uint64 { return s.At() }
	less := func(a, b uint64) bool { return a < b }
	at2 := func(s *loser.Tree[uint64, *List]) uint64 { return s.Winner().At() }
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			numCloses := 0
			close := func(s *List) {
				numCloses++
			}
			lt := loser.New(tt.args, math.MaxUint64, at, less, close)
			checkIterablesEqual(t, tt.want, lt, at, at2, less)
			if numCloses != len(tt.args) {
				t.Errorf("Expected %d closes, got %d", len(tt.args), numCloses)
			}
		})
	}
}

func TestPush(t *testing.T) {
	at := func(s *List) uint64 { return s.At() }
	less := func(a, b uint64) bool { return a < b }
	at2 := func(s *loser.Tree[uint64, *List]) uint64 { return s.Winner().At() }
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			numCloses := 0
			close := func(s *List) {
				numCloses++
			}
			lt := loser.New(nil, math.MaxUint64, at, less, close)
			for _, s := range tt.args {
				if err := lt.Push(s); err != nil {
					t.Fatalf("Push failed: %v", err)
				}
			}
			checkIterablesEqual(t, tt.want, lt, at, at2, less)
			if numCloses != len(tt.args) {
				t.Errorf("Expected %d closes, got %d", len(tt.args), numCloses)
			}
		})
	}
}

func TestInitWithErr(t *testing.T) {
	lists := []*List{
		NewList(),
		NewList(5, 6, 7, 8),
	}
	lists[0].err = testErr
	tree := loser.New(lists, math.MaxUint64, func(s *List) uint64 { return s.At() }, func(a, b uint64) bool { return a < b }, func(s *List) { s.Close() })
	if tree.Next() {
		t.Errorf("Next() should have returned false")
	}
	if tree.Err() != testErr {
		t.Errorf("Err() should have returned %v, got %v", testErr, tree.Err())
	}

	tree.Close()
	for _, l := range lists {
		assert.Equal(t, l.closed, 1, "list %+#v not closed exactly once", l)
	}

}

var testErr = errors.New("test")

func TestErrDuringNext(t *testing.T) {
	lists := []*List{
		NewList(5, 6),
		NewList(11, 12),
	}
	tree := loser.New(lists, math.MaxUint64, func(s *List) uint64 { return s.At() }, func(a, b uint64) bool { return a < b }, func(s *List) { s.Close() })

	// no error for first element
	if !tree.Next() {
		t.Errorf("Next() should have returned true")
	}
	// now error for second
	lists[0].err = testErr
	if tree.Next() {
		t.Errorf("Next() should have returned false")
	}
	if tree.Err() != testErr {
		t.Errorf("Err() should have returned %v, got %v", testErr, tree.Err())
	}
	if tree.Next() {
		t.Errorf("Next() should have returned false")
	}

	tree.Close()
	for _, l := range lists {
		assert.Equal(t, l.closed, 1, "list %+#v not closed exactly once", l)
	}
}

func TestErrInOneIterator(t *testing.T) {
	l := NewList()
	l.err = errors.New("test")

	lists := []*List{
		NewList(5, 1),
		l,
		NewList(2, 4),
	}
	tree := loser.New(lists, math.MaxUint64, func(s *List) uint64 { return s.At() }, func(a, b uint64) bool { return a < b }, func(s *List) { s.Close() })

	// error for first element
	require.False(t, tree.Next())
	assert.Equal(t, l.err, tree.Err())

	tree.Close()
	for _, l := range lists {
		assert.Equal(t, l.closed, 1, "list %+#v not closed exactly once", l)
	}
}
