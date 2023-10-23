// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/label_remover_filter_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"
	"testing"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

func TestLabelRemoverFilter(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	block3 := ulid.MustNew(3, nil)

	tests := map[string]struct {
		labels   []string
		input    map[ulid.ULID]map[string]string
		expected map[ulid.ULID]map[string]string
	}{
		"should remove configured labels": {
			labels: []string{block.HostnameLabel},
			input: map[ulid.ULID]map[string]string{
				block1: {block.HostnameLabel: "ingester-0", "foo": "user-1"},
				block2: {block.HostnameLabel: "ingester-0", "foo": "user-1"},
				block3: {block.HostnameLabel: "ingester-0", "foo": "user-1"},
			},
			expected: map[ulid.ULID]map[string]string{
				block1: {"foo": "user-1"},
				block2: {"foo": "user-1"},
				block3: {"foo": "user-1"},
			},
		},

		"should remove configured labels 2": {
			labels: []string{block.HostnameLabel, "foo"},
			input: map[ulid.ULID]map[string]string{
				block1: {block.HostnameLabel: "ingester-0", "foo": "user-1"},
				block2: {block.HostnameLabel: "ingester-0", "foo": "user-1"},
				block3: {block.HostnameLabel: "ingester-0", "foo": "user-1"},
			},
			expected: map[ulid.ULID]map[string]string{
				block1: {},
				block2: {},
				block3: {},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			metas := map[ulid.ULID]*block.Meta{}
			for id, lbls := range testData.input {
				metas[id] = &block.Meta{Labels: lbls}
			}

			f := NewLabelRemoverFilter(testData.labels)
			err := f.Filter(context.Background(), metas, nil)
			require.NoError(t, err)
			assert.Len(t, metas, len(testData.expected))

			for expectedID, expectedLbls := range testData.expected {
				assert.NotNil(t, metas[expectedID])
				assert.Equal(t, expectedLbls, metas[expectedID].Labels)
			}
		})
	}
}
