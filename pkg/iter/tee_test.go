package iter

import (
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Tee_All(t *testing.T) {
	type testCase struct {
		name    string
		items   int
		bufSize int
		iters   int
	}
	testCases := []testCase{
		{
			name:    "empty",
			items:   0,
			iters:   10,
			bufSize: 512,
		},
		{
			name:    "no iterators",
			bufSize: 512,
		},
		{
			name:    "single iterator",
			items:   1000,
			iters:   1,
			bufSize: 512,
		},
		{
			name:    "matches buffer size",
			items:   512,
			iters:   10,
			bufSize: 512,
		},
		{
			name:    "larger than buffer",
			items:   1000,
			iters:   10,
			bufSize: 512,
		},
		{
			name:    "less than buffer",
			items:   7,
			iters:   10,
			bufSize: 512,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup
			s := newSeqIterator(tc.items)
			n := make([]int, tc.iters)
			it := newTee[int](s, tc.iters, tc.bufSize)
			for i, x := range it {
				x := x
				i := i
				wg.Add(1)
				go func() {
					defer wg.Done()
					for x.Next() {
						n[i]++
					}
					assert.NoError(t, x.Close())
					assert.NoError(t, x.Err())
				}()
			}
			wg.Wait()
			for i, v := range n {
				assert.Equal(t, tc.items, v)
				assert.False(t, it[i].Next())
				assert.NoError(t, it[i].Close())
				assert.NoError(t, it[i].Err())
			}
			assert.False(t, s.Next())
			assert.NoError(t, s.Close())
			assert.NoError(t, s.Err())
		})
	}
}

func Test_Tee_Lag(t *testing.T) {
	const (
		items   = 1000
		iters   = 10
		bufSize = 512
	)
	var wg sync.WaitGroup
	s := newSeqIterator(items)
	n := make([]int, iters)
	it := newTee[int](s, iters, bufSize)
	for i, x := range it {
		x := x
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each iterator will consume i * 100 items.
			for (n[i] < (i+1)*(items/iters)) && x.Next() {
				n[i]++
			}
			assert.NoError(t, x.Close())
			assert.NoError(t, x.Err())
		}()
	}
	wg.Wait()
	for i, v := range n {
		assert.Equal(t, (i+1)*(items/iters), v)
		assert.False(t, it[i].Next())
		assert.NoError(t, it[i].Close())
		assert.NoError(t, it[i].Err())
	}
	assert.False(t, s.Next())
	assert.NoError(t, s.Close())
	assert.NoError(t, s.Err())
}

func Test_Tee_BufferReuse(t *testing.T) {
	const (
		items   = 1 << 20
		iters   = 2
		bufSize = 512
	)

	var wg sync.WaitGroup
	s := newSeqIterator(items)
	n := make([]int, iters)
	it := newTee[int](s, iters, bufSize)
	for i, x := range it {
		x := x
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var j int
			for x.Next() {
				n[i]++
				j++
				// Let others consume.
				if j%4<<10 == 0 {
					runtime.Gosched()
				}
			}
			assert.NoError(t, x.Close())
			assert.NoError(t, x.Err())
		}()
	}
	wg.Wait()

	for i, v := range n {
		assert.Equal(t, items, v)
		assert.False(t, it[i].Next())
		assert.NoError(t, it[i].Close())
		assert.NoError(t, it[i].Err())
	}
	assert.False(t, s.Next())
	assert.NoError(t, s.Close())
	assert.NoError(t, s.Err())

	// Might be flaky.
	// Typically, for the given test, the expected
	// buffer capacity is within [10K:100K].
	assert.Less(t, cap(it[0].(*tee[int]).s.v), 2*items)
}
