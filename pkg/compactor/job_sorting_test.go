// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/job_sorting_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

func TestSortJobsBySmallestRangeOldestBlocksFirst(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	block3 := ulid.MustNew(3, nil)
	block4 := ulid.MustNew(4, nil)
	block5 := ulid.MustNew(5, nil)
	block6 := ulid.MustNew(6, nil)

	tests := map[string]struct {
		input    []*Job
		expected []*Job
	}{
		"should do nothing on empty input": {
			input:    nil,
			expected: nil,
		},
		"should sort jobs by smallest range, oldest blocks first": {
			input: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block5, 40, 60), mockMetaWithMinMax(block6, 40, 80)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 10, 20)}},
			},
			expected: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 10, 20)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block5, 40, 60), mockMetaWithMinMax(block6, 40, 80)}},
			},
		},
		"split jobs are always sorted first": {
			input: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block5, 40, 60), mockMetaWithMinMax(block6, 40, 80)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}, useSplitting: false},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}, useSplitting: true},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 10, 20)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block4, 5, 50)}, useSplitting: true}, // Big splitting block. Should be sorted by minTime only.
			},
			expected: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block4, 5, 50)}, useSplitting: true},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}, useSplitting: true}, // Split job is first.
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 10, 20)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}, useSplitting: false},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block5, 40, 60), mockMetaWithMinMax(block6, 40, 80)}},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testData.expected, sortJobsBySmallestRangeOldestBlocksFirst(testData.input))
		})
	}
}

func TestSortJobsByNewestBlocksFirst(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	block3 := ulid.MustNew(3, nil)
	block4 := ulid.MustNew(4, nil)
	block5 := ulid.MustNew(5, nil)
	block6 := ulid.MustNew(6, nil)
	block7 := ulid.MustNew(7, nil)

	tests := map[string]struct {
		input    []*Job
		expected []*Job
	}{
		"should do nothing on empty input": {
			input:    nil,
			expected: nil,
		},
		"should sort jobs by newest blocks first": {
			input: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 10, 20)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block5, 40, 60), mockMetaWithMinMax(block6, 40, 80)}},
			},
			expected: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block5, 40, 60), mockMetaWithMinMax(block6, 40, 80)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block3, 10, 20), mockMetaWithMinMax(block4, 20, 30)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 10, 20)}},
			},
		},
		"should give precedence to smaller time ranges in case of multiple jobs with the same max time": {
			input: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 20, 30), mockMetaWithMinMax(block3, 30, 40)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block4, 30, 40), mockMetaWithMinMax(block5, 30, 40)}},
			},
			expected: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block4, 30, 40), mockMetaWithMinMax(block5, 30, 40)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 20, 30), mockMetaWithMinMax(block3, 30, 40)}},
			},
		},
		"should give precedence to newest blocks over smaller time ranges": {
			input: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 20, 30), mockMetaWithMinMax(block3, 30, 40)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block6, 10, 20), mockMetaWithMinMax(block7, 10, 20)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block4, 10, 30), mockMetaWithMinMax(block5, 20, 30)}},
			},
			expected: []*Job{
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block1, 10, 20), mockMetaWithMinMax(block2, 20, 30), mockMetaWithMinMax(block3, 30, 40)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block4, 10, 30), mockMetaWithMinMax(block5, 20, 30)}},
				{metasByMinTime: []*block.Meta{mockMetaWithMinMax(block6, 10, 20), mockMetaWithMinMax(block7, 10, 20)}},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			actual := sortJobsByNewestBlocksFirst(testData.input)
			assert.Equal(t, testData.expected, actual)

			// Print for debugging.
			t.Log("sorted jobs:")
			for _, job := range actual {
				t.Logf("- %s", job.String())
			}
		})
	}
}

func mockMetaWithMinMax(id ulid.ULID, minTime, maxTime int64) *block.Meta {
	return &block.Meta{
		ULID:    id,
		MinTime: model.Time(minTime),
		MaxTime: model.Time(maxTime),
	}
}
