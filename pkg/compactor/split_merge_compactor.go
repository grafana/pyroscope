// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_compactor.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
package compactor

import (
	"context"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

func splitAndMergeGrouperFactory(_ context.Context, cfg Config, cfgProvider ConfigProvider, userID string, logger log.Logger, _ prometheus.Registerer) Grouper {
	return NewSplitAndMergeGrouper(
		userID,
		cfg.BlockRanges.ToMilliseconds(),
		uint32(cfgProvider.CompactorSplitAndMergeShards(userID)),
		uint32(cfgProvider.CompactorSplitAndMergeStageSize(userID)),
		uint32(cfgProvider.CompactorSplitGroups(userID)),
		logger)
}

func splitAndMergePlannerFactory(cfg Config) Planner {
	return NewSplitAndMergePlanner(cfg.BlockRanges.ToMilliseconds())
}

func splitAndMergeCompactorFactory(_ context.Context, cfg Config, cfgProvider ConfigProvider, userID string, logger log.Logger, metrics *CompactorMetrics) (Compactor, error) {
	splitBy := getCompactionSplitBy(cfg.CompactionSplitBy)
	if splitBy == nil {
		return nil, errInvalidCompactionSplitBy
	}
	return &BlockCompactor{
		blockOpenConcurrency: cfg.MaxOpeningBlocksConcurrency,
		downsamplerEnabled:   cfg.DownsamplerEnabled && cfgProvider.CompactorDownsamplerEnabled(userID),
		splitBy:              splitBy,
		logger:               logger,
		metrics:              metrics,
	}, nil
}

// configureSplitAndMergeCompactor updates the provided configuration injecting the split-and-merge compactor.
func configureSplitAndMergeCompactor(cfg *Config) {
	cfg.BlocksGrouperFactory = splitAndMergeGrouperFactory
	cfg.BlocksCompactorFactory = splitAndMergeCompactorFactory
	cfg.BlocksPlannerFactory = splitAndMergePlannerFactory
}
