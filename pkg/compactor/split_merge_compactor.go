// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_compactor.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
package compactor

import (
	"context"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb"
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
	// We don't need to customise the TSDB compactor so we're just using the Prometheus one.
	compactor, err := tsdb.NewLeveledCompactor(ctx, reg, logger, cfg.BlockRanges.ToMilliseconds(), nil, nil, true)
	if err != nil {
		return nil, nil, err
	}

	opts := tsdb.DefaultLeveledCompactorConcurrencyOptions()
	opts.MaxOpeningBlocks = cfg.MaxOpeningBlocksConcurrency
	opts.MaxClosingBlocks = cfg.MaxClosingBlocksConcurrency
	opts.SymbolsFlushersCount = cfg.SymbolsFlushersConcurrency

	compactor.SetConcurrencyOptions(opts)

	planner := NewSplitAndMergePlanner(cfg.BlockRanges.ToMilliseconds())
	return compactor, planner, nil
}

// configureSplitAndMergeCompactor updates the provided configuration injecting the split-and-merge compactor.
func configureSplitAndMergeCompactor(cfg *Config) {
	cfg.BlocksGrouperFactory = splitAndMergeGrouperFactory
	cfg.BlocksCompactorFactory = splitAndMergeCompactorFactory
}
