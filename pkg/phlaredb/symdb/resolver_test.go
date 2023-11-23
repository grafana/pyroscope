package symdb

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_Resolver_Unreleased_Failed_Partition(t *testing.T) {
	s := newBlockSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	defer s.teardown()
	ctx, cancel := context.WithCancel(context.Background())
	// Pass canceled context to make partition initialization to fail.
	cancel()

	r := NewResolver(ctx, s.reader)
	r.AddSamples(0, s.indexed[0][0].Samples)
	_, err := r.Tree()
	require.ErrorIs(t, err, context.Canceled)
	r.Release()

	// This time we pass normal context.
	r = NewResolver(context.Background(), s.reader)
	r.AddSamples(0, s.indexed[0][0].Samples)
	_, err = r.Tree()
	require.NoError(t, err)
	r.Release()
}

func Test_Resolver_Error_Propagation(t *testing.T) {
	m := new(mockSymbolsReader)
	m.On("Partition", mock.Anything, mock.Anything).Return(nil, io.EOF).Once()
	r := NewResolver(context.Background(), m)
	r.AddSamples(0, schemav1.Samples{})
	_, err := r.Tree()
	require.ErrorIs(t, err, io.EOF)
	r.Release()
}

func Test_Resolver_Cancellation(t *testing.T) {
	s := newBlockSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	defer s.teardown()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		workers    = 10
		iterations = 10
		depth      = 5
	)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				for d := 0; d < depth; d++ {
					func() {
						r := NewResolver(contextCancelAfter(ctx, int64(d)), s.reader)
						defer r.Release()
						r.AddSamples(0, s.indexed[0][0].Samples)
						_, _ = r.Tree()
					}()
				}
			}
		}()
	}

	wg.Wait()
}

type mockSymbolsReader struct{ mock.Mock }

func (m *mockSymbolsReader) Partition(ctx context.Context, partition uint64) (PartitionReader, error) {
	args := m.Called(ctx, partition)
	r, _ := args.Get(0).(PartitionReader)
	return r, args.Error(1)
}

func (m *mockSymbolsReader) Load(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

type fakeContext struct {
	context.Context
	once sync.Once
	ch   chan struct{}
	c    atomic.Int64
	n    int64
}

func contextCancelAfter(ctx context.Context, n int64) context.Context {
	return &fakeContext{
		ch:      make(chan struct{}),
		Context: ctx,
		n:       n,
	}
}

func (f *fakeContext) Done() <-chan struct{} {
	if f.c.Add(1) > f.n {
		f.once.Do(func() {
			close(f.ch)
		})
	}
	return f.ch
}

func (f *fakeContext) Err() error {
	if f.c.Load() > f.n {
		return context.Canceled
	}
	return f.Context.Err()
}
