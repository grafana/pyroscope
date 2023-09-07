// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/bucket_compactor_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	"github.com/grafana/mimir/pkg/storage/tsdb/block"
	"github.com/grafana/mimir/pkg/util/extprom"
)

func TestGroupKey(t *testing.T) {
	for _, tcase := range []struct {
		input    block.ThanosMeta
		expected string
	}{
		{
			input:    block.ThanosMeta{},
			expected: "0@17241709254077376921",
		},
		{
			input: block.ThanosMeta{
				Labels:     map[string]string{},
				Downsample: block.ThanosDownsample{Resolution: 0},
			},
			expected: "0@17241709254077376921",
		},
		{
			input: block.ThanosMeta{
				Labels:     map[string]string{"foo": "bar", "foo1": "bar2"},
				Downsample: block.ThanosDownsample{Resolution: 0},
			},
			expected: "0@2124638872457683483",
		},
		{
			input: block.ThanosMeta{
				Labels:     map[string]string{`foo/some..thing/some.thing/../`: `a_b_c/bar-something-a\metric/a\x`},
				Downsample: block.ThanosDownsample{Resolution: 0},
			},
			expected: "0@16590761456214576373",
		},
	} {
		if ok := t.Run("", func(t *testing.T) {
			assert.Equal(t, tcase.expected, DefaultGroupKey(tcase.input))
		}); !ok {
			return
		}
	}
}

func TestGroupMaxMinTime(t *testing.T) {
	g := &Job{
		metasByMinTime: []*block.Meta{
			{BlockMeta: tsdb.BlockMeta{MinTime: 0, MaxTime: 10}},
			{BlockMeta: tsdb.BlockMeta{MinTime: 1, MaxTime: 20}},
			{BlockMeta: tsdb.BlockMeta{MinTime: 2, MaxTime: 30}},
		},
	}

	assert.Equal(t, int64(0), g.MinTime())
	assert.Equal(t, int64(30), g.MaxTime())
}

func TestBucketCompactor_FilterOwnJobs(t *testing.T) {
	jobsFn := func() []*Job {
		return []*Job{
			NewJob("user", "key1", labels.EmptyLabels(), 0, false, 0, ""),
			NewJob("user", "key2", labels.EmptyLabels(), 0, false, 0, ""),
			NewJob("user", "key3", labels.EmptyLabels(), 0, false, 0, ""),
			NewJob("user", "key4", labels.EmptyLabels(), 0, false, 0, ""),
		}
	}

	tests := map[string]struct {
		ownJob       ownCompactionJobFunc
		expectedJobs int
	}{
		"should return all planned jobs if the compactor instance owns all of them": {
			ownJob: func(job *Job) (bool, error) {
				return true, nil
			},
			expectedJobs: 4,
		},
		"should return no jobs if the compactor instance owns none of them": {
			ownJob: func(job *Job) (bool, error) {
				return false, nil
			},
			expectedJobs: 0,
		},
		"should return some jobs if the compactor instance owns some of them": {
			ownJob: func() ownCompactionJobFunc {
				count := 0
				return func(job *Job) (bool, error) {
					count++
					return count%2 == 0, nil
				}
			}(),
			expectedJobs: 2,
		},
	}

	m := NewBucketCompactorMetrics(promauto.With(nil).NewCounter(prometheus.CounterOpts{}), nil)
	for testName, testCase := range tests {
		t.Run(testName, func(t *testing.T) {
			bc, err := NewBucketCompactor(log.NewNopLogger(), nil, nil, nil, nil, "", nil, 2, false, testCase.ownJob, nil, 0, 4, m)
			require.NoError(t, err)

			res, err := bc.filterOwnJobs(jobsFn())

			require.NoError(t, err)
			assert.Len(t, res, testCase.expectedJobs)
		})
	}
}

func TestBlockMaxTimeDeltas(t *testing.T) {
	j1 := NewJob("user", "key1", labels.EmptyLabels(), 0, false, 0, "")
	require.NoError(t, j1.AppendMeta(&block.Meta{
		BlockMeta: tsdb.BlockMeta{
			MinTime: 1500002700159,
			MaxTime: 1500002800159,
		},
	}))

	j2 := NewJob("user", "key2", labels.EmptyLabels(), 0, false, 0, "")
	require.NoError(t, j2.AppendMeta(&block.Meta{
		BlockMeta: tsdb.BlockMeta{
			MinTime: 1500002600159,
			MaxTime: 1500002700159,
		},
	}))
	require.NoError(t, j2.AppendMeta(&block.Meta{
		BlockMeta: tsdb.BlockMeta{
			MinTime: 1500002700159,
			MaxTime: 1500002800159,
		},
	}))

	metrics := NewBucketCompactorMetrics(promauto.With(nil).NewCounter(prometheus.CounterOpts{}), nil)
	now := time.UnixMilli(1500002900159)
	bc, err := NewBucketCompactor(log.NewNopLogger(), nil, nil, nil, nil, "", nil, 2, false, nil, nil, 0, 4, metrics)
	require.NoError(t, err)

	deltas := bc.blockMaxTimeDeltas(now, []*Job{j1, j2})
	assert.Equal(t, []float64{100, 200, 100}, deltas)
}

func TestNoCompactionMarkFilter(t *testing.T) {
	ctx := context.Background()
	// Use bucket with global markers to make sure that our custom filters work correctly.
	bkt := block.BucketWithGlobalMarkers(objstore.NewInMemBucket())

	block1 := ulid.MustParse("01DTVP434PA9VFXSW2JK000001") // No mark file.
	block2 := ulid.MustParse("01DTVP434PA9VFXSW2JK000002") // Marked for no-compaction
	block3 := ulid.MustParse("01DTVP434PA9VFXSW2JK000003") // Has wrong version of marker file.
	block4 := ulid.MustParse("01DTVP434PA9VFXSW2JK000004") // Has invalid marker file.
	block5 := ulid.MustParse("01DTVP434PA9VFXSW2JK000005") // No mark file.

	for name, testFn := range map[string]func(t *testing.T, synced block.GaugeVec){
		"filter with no deletion of blocks marked for no-compaction": func(t *testing.T, synced block.GaugeVec) {
			metas := map[ulid.ULID]*block.Meta{
				block1: blockMeta(block1.String(), 100, 200, nil),
				block2: blockMeta(block2.String(), 200, 300, nil), // Has no-compaction marker.
				block4: blockMeta(block4.String(), 400, 500, nil), // Invalid marker is still a marker, and block will be in NoCompactMarkedBlocks.
				block5: blockMeta(block5.String(), 500, 600, nil),
			}

			f := NewNoCompactionMarkFilter(objstore.WithNoopInstr(bkt), false)
			require.NoError(t, f.Filter(ctx, metas, synced))

			require.Contains(t, metas, block1)
			require.Contains(t, metas, block2)
			require.Contains(t, metas, block4)
			require.Contains(t, metas, block5)

			require.Len(t, f.NoCompactMarkedBlocks(), 2)
			require.Contains(t, f.NoCompactMarkedBlocks(), block2, block4)

			assert.Equal(t, 2.0, testutil.ToFloat64(synced.WithLabelValues(block.MarkedForNoCompactionMeta)))
		},
		"filter with deletion enabled": func(t *testing.T, synced block.GaugeVec) {
			metas := map[ulid.ULID]*block.Meta{
				block1: blockMeta(block1.String(), 100, 200, nil),
				block2: blockMeta(block2.String(), 300, 300, nil), // Has no-compaction marker.
				block4: blockMeta(block4.String(), 400, 500, nil), // Marker with invalid syntax is ignored.
				block5: blockMeta(block5.String(), 500, 600, nil),
			}

			f := NewNoCompactionMarkFilter(objstore.WithNoopInstr(bkt), true)
			require.NoError(t, f.Filter(ctx, metas, synced))

			require.Contains(t, metas, block1)
			require.NotContains(t, metas, block2) // block2 was removed from metas.
			require.NotContains(t, metas, block4) // block4 has invalid marker, but we don't check for marker content.
			require.Contains(t, metas, block5)

			require.Len(t, f.NoCompactMarkedBlocks(), 2)
			require.Contains(t, f.NoCompactMarkedBlocks(), block2)
			require.Contains(t, f.NoCompactMarkedBlocks(), block4)

			assert.Equal(t, 2.0, testutil.ToFloat64(synced.WithLabelValues(block.MarkedForNoCompactionMeta)))
		},
		"filter with deletion enabled, but canceled context": func(t *testing.T, synced block.GaugeVec) {
			metas := map[ulid.ULID]*block.Meta{
				block1: blockMeta(block1.String(), 100, 200, nil),
				block2: blockMeta(block2.String(), 200, 300, nil),
				block3: blockMeta(block3.String(), 300, 400, nil),
				block4: blockMeta(block4.String(), 400, 500, nil),
				block5: blockMeta(block5.String(), 500, 600, nil),
			}

			canceledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			f := NewNoCompactionMarkFilter(objstore.WithNoopInstr(bkt), true)
			require.Error(t, f.Filter(canceledCtx, metas, synced))

			require.Contains(t, metas, block1)
			require.Contains(t, metas, block2)
			require.Contains(t, metas, block3)
			require.Contains(t, metas, block4)
			require.Contains(t, metas, block5)

			require.Empty(t, f.NoCompactMarkedBlocks())
			assert.Equal(t, 0.0, testutil.ToFloat64(synced.WithLabelValues(block.MarkedForNoCompactionMeta)))
		},
		"filtering block with wrong marker version": func(t *testing.T, synced block.GaugeVec) {
			metas := map[ulid.ULID]*block.Meta{
				block3: blockMeta(block3.String(), 300, 300, nil), // Has compaction marker with invalid version, but Filter doesn't check for that.
			}

			f := NewNoCompactionMarkFilter(objstore.WithNoopInstr(bkt), true)
			err := f.Filter(ctx, metas, synced)
			require.NoError(t, err)
			require.Empty(t, metas)

			assert.Equal(t, 1.0, testutil.ToFloat64(synced.WithLabelValues(block.MarkedForNoCompactionMeta)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Block 2 is marked for no-compaction.
			require.NoError(t, block.MarkForNoCompact(ctx, log.NewNopLogger(), bkt, block2, block.OutOfOrderChunksNoCompactReason, "details...", promauto.With(nil).NewCounter(prometheus.CounterOpts{})))
			// Block 3 has marker with invalid version
			require.NoError(t, bkt.Upload(ctx, block3.String()+"/no-compact-mark.json", strings.NewReader(`{"id":"`+block3.String()+`","version":100,"details":"details","no_compact_time":1637757932,"reason":"reason"}`)))
			// Block 4 has marker with invalid JSON syntax
			require.NoError(t, bkt.Upload(ctx, block4.String()+"/no-compact-mark.json", strings.NewReader(`invalid json`)))

			synced := extprom.NewTxGaugeVec(nil, prometheus.GaugeOpts{Name: "synced", Help: "Number of block metadata synced"},
				[]string{"state"}, []string{block.MarkedForNoCompactionMeta},
			)

			testFn(t, synced)
		})
	}
}

func TestConvertCompactionResultToForEachJobs(t *testing.T) {
	ulid1 := ulid.MustNew(1, nil)
	ulid2 := ulid.MustNew(2, nil)

	res := convertCompactionResultToForEachJobs([]ulid.ULID{{}, ulid1, {}, ulid2, {}}, true, log.NewNopLogger())
	require.Len(t, res, 2)
	require.Equal(t, ulidWithShardIndex{ulid: ulid1, shardIndex: 1}, res[0])
	require.Equal(t, ulidWithShardIndex{ulid: ulid2, shardIndex: 3}, res[1])
}

func TestCompactedBlocksTimeRangeVerification(t *testing.T) {
	const (
		sourceMinTime = 1000
		sourceMaxTime = 2500
	)

	tests := map[string]struct {
		compactedBlockMinTime int64
		compactedBlockMaxTime int64
		shouldErr             bool
		expectedErrMsg        string
	}{
		"should pass with minTime and maxTime matching the source blocks": {
			compactedBlockMinTime: sourceMinTime,
			compactedBlockMaxTime: sourceMaxTime,
			shouldErr:             false,
		},
		"should fail with compacted block minTime < source minTime": {
			compactedBlockMinTime: sourceMinTime - 500,
			compactedBlockMaxTime: sourceMaxTime,
			shouldErr:             true,
			expectedErrMsg:        fmt.Sprintf("compacted block minTime %d is before source minTime %d", sourceMinTime-500, sourceMinTime),
		},
		"should fail with compacted block maxTime > source maxTime": {
			compactedBlockMinTime: sourceMinTime,
			compactedBlockMaxTime: sourceMaxTime + 500,
			shouldErr:             true,
			expectedErrMsg:        fmt.Sprintf("compacted block maxTime %d is after source maxTime %d", sourceMaxTime+500, sourceMaxTime),
		},
		"should fail due to minTime and maxTime not found": {
			compactedBlockMinTime: sourceMinTime + 250,
			compactedBlockMaxTime: sourceMaxTime - 250,
			shouldErr:             true,
			expectedErrMsg:        fmt.Sprintf("compacted block(s) do not contain minTime %d and maxTime %d from the source blocks", sourceMinTime, sourceMaxTime),
		},
	}

	for testName, testData := range tests {
		testData := testData // Prevent loop variable being captured by func literal
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			compactedBlock1, err := block.CreateBlock(
				context.Background(), tempDir,
				[]labels.Labels{
					labels.FromStrings("test", "foo", "a", "1"),
					labels.FromStrings("test", "foo", "a", "2"),
					labels.FromStrings("test", "foo", "a", "3"),
				}, 10, testData.compactedBlockMinTime, testData.compactedBlockMinTime+500, labels.EmptyLabels())
			require.NoError(t, err)

			compactedBlock2, err := block.CreateBlock(
				context.Background(), tempDir,
				[]labels.Labels{
					labels.FromStrings("test", "foo", "a", "1"),
					labels.FromStrings("test", "foo", "a", "2"),
					labels.FromStrings("test", "foo", "a", "3"),
				}, 10, testData.compactedBlockMaxTime-500, testData.compactedBlockMaxTime, labels.EmptyLabels())
			require.NoError(t, err)

			err = verifyCompactedBlocksTimeRanges([]ulid.ULID{compactedBlock1, compactedBlock2}, sourceMinTime, sourceMaxTime, tempDir)
			if testData.shouldErr {
				require.ErrorContains(t, err, testData.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
