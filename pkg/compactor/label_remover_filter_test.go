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

	mimir_tsdb "github.com/grafana/mimir/pkg/storage/tsdb"
	"github.com/grafana/mimir/pkg/storage/tsdb/block"
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
			labels: []string{mimir_tsdb.DeprecatedIngesterIDExternalLabel},
			input: map[ulid.ULID]map[string]string{
				block1: {mimir_tsdb.DeprecatedIngesterIDExternalLabel: "ingester-0", mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
				block2: {mimir_tsdb.DeprecatedIngesterIDExternalLabel: "ingester-0", mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
				block3: {mimir_tsdb.DeprecatedIngesterIDExternalLabel: "ingester-0", mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
			},
			expected: map[ulid.ULID]map[string]string{
				block1: {mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
				block2: {mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
				block3: {mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
			},
		},

		"should remove configured labels 2": {
			labels: []string{mimir_tsdb.DeprecatedIngesterIDExternalLabel, mimir_tsdb.DeprecatedTenantIDExternalLabel},
			input: map[ulid.ULID]map[string]string{
				block1: {mimir_tsdb.DeprecatedIngesterIDExternalLabel: "ingester-0", mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
				block2: {mimir_tsdb.DeprecatedIngesterIDExternalLabel: "ingester-0", mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
				block3: {mimir_tsdb.DeprecatedIngesterIDExternalLabel: "ingester-0", mimir_tsdb.DeprecatedTenantIDExternalLabel: "user-1"},
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
				metas[id] = &block.Meta{Thanos: block.ThanosMeta{Labels: lbls}}
			}

			f := NewLabelRemoverFilter(testData.labels)
			err := f.Filter(context.Background(), metas, nil)
			require.NoError(t, err)
			assert.Len(t, metas, len(testData.expected))

			for expectedID, expectedLbls := range testData.expected {
				assert.NotNil(t, metas[expectedID])
				assert.Equal(t, expectedLbls, metas[expectedID].Thanos.Labels)
			}
		})
	}
}
