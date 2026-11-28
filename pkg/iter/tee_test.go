package iter

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Tee_PrivateBatches(t *testing.T) {
	for _, size := range []int{1, 7, 128, 129, 512} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			const count = 4099
			values := make([]int, count)
			for i := range values {
				values[i] = i
			}
			iters := newTee[int](NewSliceIterator(values), 3, size)
			var wg sync.WaitGroup
			for k, it := range iters {
				wg.Add(1)
				go func(k int, it Iterator[int]) {
					defer wg.Done()
					limit := count
					if k == 2 {
						limit = 137 // Close with a partially consumed private batch.
					}
					for i := 0; i < limit; i++ {
						if !assert.True(t, it.Next()) {
							return
						}
						assert.Equal(t, i, it.At())
						assert.Equal(t, i, it.At())
						if k == 1 && i%17 == 0 {
							runtime.Gosched()
						}
					}
					if k != 2 {
						assert.False(t, it.Next())
					}
					assert.NoError(t, it.Close())
					assert.NoError(t, it.Close())
					assert.False(t, it.Next())
				}(k, it)
			}
			wg.Wait()
		})
	}
}

func Test_Tee_CleanPointerRetention(t *testing.T) {
	for _, closeEarly := range []bool{false, true} {
		t.Run(fmt.Sprint(closeEarly), func(t *testing.T) {
			values := make([]*int, 300)
			for i := range values {
				v := i
				values[i] = &v
			}
			it := newTee[*int](NewSliceIterator(values), 1, 256)[0].(*tee[*int])
			assert.True(t, it.Next())
			assert.Nil(t, it.scratch[0])
			if closeEarly {
				assert.NoError(t, it.Close())
			} else {
				for it.Next() {
				}
			}
			assert.Nil(t, it.At())
			for _, v := range it.scratch {
				assert.Nil(t, v)
			}
			if !closeEarly {
				for _, v := range it.s.v[len(it.s.v):cap(it.s.v)] {
					assert.Nil(t, v)
				}
			}
		})
	}
}

func Test_Tee_Error(t *testing.T) {
	want := errors.New("source failed")
	for _, it := range TeeN[int](NewErrIterator[int](want), 2) {
		assert.False(t, it.Next())
		assert.ErrorIs(t, it.Err(), want)
		assert.NoError(t, it.Close())
	}
}

func BenchmarkTee_Int64_Concurrent(b *testing.B) {
	benchmarkTeeConcurrent(b, make([]int64, 16384), 3)
}

func BenchmarkTee_Struct64B_Concurrent(b *testing.B) {
	benchmarkTeeConcurrent(b, make([][8]int64, 16384), 2)
}

func benchmarkTeeConcurrent[T any](b *testing.B, values []T, consumers int) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for _, it := range TeeN[T](NewSliceIterator(values), consumers) {
			wg.Add(1)
			go func(it Iterator[T]) {
				defer wg.Done()
				for it.Next() {
					_ = it.At()
				}
				_ = it.Close()
			}(it)
		}
		wg.Wait()
	}
}

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
