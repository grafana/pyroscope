package symbolizer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func TestResolveSizeLimitExceeded(t *testing.T) {
	limits := validation.MockOverrides(func(_ *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.Symbolizer.MaxSymbolSizeBytes = 4
		tenantLimits["tenant-limited"] = l
	})
	s, mockClient, mockBucket := newSymbolizerTest(t, &symbolizerInputs{Limits: limits})

	mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant-limited", "build-id")).Return(nil, fmt.Errorf("not found")).Once()
	mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(openTestFile(t), nil).Once()

	ctx := tenant.InjectTenantID(context.Background(), "tenant-limited")
	frames, err := s.Resolve(ctx, "build-id", "some-binary", []uint64{0x1500})

	require.NoError(t, err)
	require.Len(t, frames, 1)
	require.Nil(t, frames[0])
}

func TestResolveContextCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		s, _, _ := newSymbolizerTest(t, nil)
		ctx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), "tenant"))
		cancel()

		frames, err := s.Resolve(ctx, "build-id", "binary", []uint64{0x1500})

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, frames)
	})

	t.Run("canceled mid-fetch", func(t *testing.T) {
		s, mockClient, mockBucket := newSymbolizerTest(t, nil)
		mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", "build-id")).Return(nil, fmt.Errorf("not found")).Once()

		fetchStarted := make(chan struct{})
		mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(
			func(ctx context.Context, buildID string) (io.ReadCloser, error) {
				close(fetchStarted)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		).Once()

		ctx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), "tenant"))
		type result struct {
			frames [][]lidia.SourceInfoFrame
			err    error
		}
		resCh := make(chan result, 1)
		go func() {
			frames, err := s.Resolve(ctx, "build-id", "binary", []uint64{0x1500})
			resCh <- result{frames, err}
		}()

		<-fetchStarted
		cancel()
		res := <-resCh

		require.Error(t, res.err)
		require.ErrorIs(t, res.err, context.Canceled)
		require.Nil(t, res.frames)
	})

	t.Run("canceled mid-fetch with successful fetch", func(t *testing.T) {
		s, mockClient, mockBucket := newSymbolizerTest(t, nil)
		mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", "build-id")).Return(nil, fmt.Errorf("not found")).Once()

		// The deduplicated fetch is detached from caller cancellation, so it
		// can complete successfully after the caller's context is done; the
		// canceled caller must still get an error, not the fetched result.
		fetchStarted := make(chan struct{})
		mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(
			func(ctx context.Context, buildID string) (io.ReadCloser, error) {
				close(fetchStarted)
				<-ctx.Done()
				return openTestFile(t), nil
			},
		).Once()
		mockBucket.On("Upload", mock.Anything, lidiaObjectPath("tenant", "build-id"), mock.Anything).Return(nil).Once()

		ctx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), "tenant"))
		type result struct {
			frames [][]lidia.SourceInfoFrame
			err    error
		}
		resCh := make(chan result, 1)
		go func() {
			frames, err := s.Resolve(ctx, "build-id", "binary", []uint64{0x1500})
			resCh <- result{frames, err}
		}()

		<-fetchStarted
		cancel()
		res := <-resCh

		require.Error(t, res.err)
		require.ErrorIs(t, res.err, context.Canceled)
		require.Nil(t, res.frames)
	})

	t.Run("deadline exceeded mid-fetch", func(t *testing.T) {
		s, mockClient, mockBucket := newSymbolizerTest(t, nil)
		mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", "build-id")).Return(nil, fmt.Errorf("not found")).Once()

		mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(
			func(ctx context.Context, buildID string) (io.ReadCloser, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		).Once()

		ctx, cancel := context.WithTimeout(tenant.InjectTenantID(context.Background(), "tenant"), 1*time.Millisecond)
		defer cancel()

		frames, err := s.Resolve(ctx, "build-id", "binary", []uint64{0x1500})

		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Nil(t, frames)
	})

	t.Run("foreign context error degrades to fallback", func(t *testing.T) {
		s, mockClient, mockBucket := newSymbolizerTest(t, nil)
		mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", "build-id")).Return(nil, fmt.Errorf("not found")).Once()
		// The deduplicated debuginfod fetch is shared with other callers and
		// can surface another caller's cancellation; with this caller's
		// context alive it must degrade to unresolved slots like any other
		// fetch failure.
		mockClient.On("FetchDebuginfo", mock.Anything, "build-id").Return(nil, context.Canceled).Once()

		frames, err := s.Resolve(tenant.InjectTenantID(context.Background(), "tenant"), "build-id", "binary", []uint64{0x1500})

		require.NoError(t, err)
		require.Len(t, frames, 1)
		require.Nil(t, frames[0])
	})
}

func TestResolveInvalidBuildID(t *testing.T) {
	s, mockClient, mockBucket := newSymbolizerTest(t, nil)
	ctx := tenant.InjectTenantID(context.Background(), "tenant")

	frames, err := s.Resolve(ctx, "../traversal", "binary", []uint64{0x1500, 0x3c5a})

	require.NoError(t, err)
	require.Len(t, frames, 2)
	require.Nil(t, frames[0])
	require.Nil(t, frames[1])
	require.Equal(t, float64(1), testutil.ToFloat64(s.metrics.debugSymbolResolutionErrors.WithLabelValues("invalid_build_id")))

	// No mockClient/mockBucket expectations configured above: AssertExpectations here
	// (and the mocks' own t.Cleanup) prove Resolve rejected the malformed buildID before
	// touching the bucket or debuginfod.
	mockClient.AssertExpectations(t)
	mockBucket.AssertExpectations(t)
}

func TestSymbolizePprofPropagatesContextCancellation(t *testing.T) {
	s, _, _ := newSymbolizerTest(t, nil)
	profile := &googlev1.Profile{
		Mapping: []*googlev1.Mapping{{
			BuildId:     1,
			MemoryStart: 0x0,
			MemoryLimit: 0x1000000,
		}},
		Location:    []*googlev1.Location{{Id: 1, MappingId: 1, Address: 0x1500}},
		StringTable: []string{"", "build-id"},
	}

	ctx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), "tenant"))
	cancel()

	err := s.SymbolizePprof(ctx, profile)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSymbolizeMappingsConcurrentFanOut(t *testing.T) {
	elfFile := openTestFile(t)
	elfData, err := io.ReadAll(elfFile)
	require.NoError(t, err)
	require.NoError(t, elfFile.Close())

	s, mockClient, mockBucket := newSymbolizerTest(t, nil)
	s.cfg.MaxDebuginfodConcurrency = 4

	// addrs per build ID; 0x1500 resolves to "main", 0x3c5a resolves to "atoll_b"
	// (see testdata/symbols.debug, also used by TestSymbolizePprof).
	buildIDAddrs := map[string][]uint64{
		"build-id-1": {0x1500, 0x3c5a},
		"build-id-2": {0x1500},
		"build-id-3": {0x1500},
	}
	buildIDs := []string{"build-id-1", "build-id-2", "build-id-3"}

	fetchStarted := make(chan struct{}, len(buildIDs))
	release := make(chan struct{})
	blockUntilReleased := func(context.Context, string) (io.ReadCloser, error) {
		fetchStarted <- struct{}{}
		<-release
		return io.NopCloser(bytes.NewReader(elfData)), nil
	}

	stringTable := []string{""}
	mappings := make([]*googlev1.Mapping, len(buildIDs))
	var locations []*googlev1.Location
	nextLocID := uint64(1)

	for i, buildID := range buildIDs {
		buildIDIdx := int64(len(stringTable))
		stringTable = append(stringTable, buildID)

		mappings[i] = &googlev1.Mapping{
			BuildId:     buildIDIdx,
			MemoryStart: 0x0,
			MemoryLimit: 0x1000000,
		}
		mappingID := uint64(i + 1)

		for _, addr := range buildIDAddrs[buildID] {
			locations = append(locations, &googlev1.Location{
				Id:        nextLocID,
				MappingId: mappingID,
				Address:   addr,
			})
			nextLocID++
		}

		mockBucket.On("Get", mock.Anything, lidiaObjectPath("tenant", buildID)).Return(nil, fmt.Errorf("not found")).Once()
		mockClient.On("FetchDebuginfo", mock.Anything, buildID).Return(blockUntilReleased).Once()
		mockBucket.On("Upload", mock.Anything, lidiaObjectPath("tenant", buildID), mock.Anything).Return(nil).Once()
	}

	profile := &googlev1.Profile{
		Mapping:     mappings,
		Location:    locations,
		StringTable: stringTable,
	}

	ctx := tenant.InjectTenantID(context.Background(), "tenant")
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.SymbolizePprof(ctx, profile)
	}()

	for range buildIDs {
		<-fetchStarted
	}
	close(release)

	require.NoError(t, <-errCh)

	for _, mapping := range mappings {
		require.True(t, mapping.HasFunctions)
	}
	assertLocationHasFunction(t, profile, locations[0], "main", "main")
	assertLocationHasFunction(t, profile, locations[1], "atoll_b", "atoll_b")
	assertLocationHasFunction(t, profile, locations[2], "main", "main")
	assertLocationHasFunction(t, profile, locations[3], "main", "main")
}
