// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_compactor.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
package compactor

import (
	"context"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/prometheus/client_golang/prometheus"
)

func splitAndMergeGrouperFactory(_ context.Context, cfg Config, cfgProvider ConfigProvider, userID string, logger log.Logger, _ prometheus.Registerer) Grouper {
	return NewSplitAndMergeGrouper(
		userID,
		cfg.BlockRanges.ToMilliseconds(),
		uint32(cfgProvider.CompactorSplitAndMergeShards(userID)),
		uint32(cfgProvider.CompactorSplitGroups(userID)),
		logger)
}

func splitAndMergeCompactorFactory(ctx context.Context, cfg Config, logger log.Logger, reg prometheus.Registerer) (Compactor, Planner, error) {
	splitBy := getCompactionSplitBy(cfg.CompactionSplitBy)
	if splitBy == nil {
		return nil, nil, errInvalidCompactionSplitBy
	}
	hash := xxhash.New()
	return &BlockCompactor{
		blockOpenConcurrency: cfg.MaxOpeningBlocksConcurrency,
		splitBy: func(r phlaredb.ProfileRow, shardsCount uint64) uint64 {
			hash.Reset()
			_, _ = hash.WriteString(r.Labels.Get(phlaremodel.LabelNameProfileType))
			_, _ = hash.WriteString("_")
			_, _ = hash.WriteString(r.Labels.Get(phlaremodel.LabelNameServiceName))
			return hash.Sum64() % shardsCount
		},
		logger:  logger,
		metrics: newCompactorMetrics(reg),
	}, NewSplitAndMergePlanner(cfg.BlockRanges.ToMilliseconds()), nil
}

// configureSplitAndMergeCompactor updates the provided configuration injecting the split-and-merge compactor.
func configureSplitAndMergeCompactor(cfg *Config) {
	cfg.BlocksGrouperFactory = splitAndMergeGrouperFactory
	cfg.BlocksCompactorFactory = splitAndMergeCompactorFactory
}
