// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/label_remover_filter.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"

	"github.com/oklog/ulid"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

type LabelRemoverFilter struct {
	labels []string
}

// NewLabelRemoverFilter creates a LabelRemoverFilter.
func NewLabelRemoverFilter(labels []string) *LabelRemoverFilter {
	return &LabelRemoverFilter{labels: labels}
}

// Filter modifies external labels of existing blocks, removing given labels from the metadata of blocks that have it.
func (f *LabelRemoverFilter) Filter(_ context.Context, metas map[ulid.ULID]*block.Meta, _ block.GaugeVec) error {
	for _, meta := range metas {
		for _, l := range f.labels {
			delete(meta.Labels, l)
		}
	}

	return nil
}
