package metastore

import (
	"flag"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

type CompactionConfig struct {
	JobLeaseDuration time.Duration `yaml:"job_lease_duration"`
	JobMaxFailures   int           `yaml:"job_max_failures"`
}

func (cfg *CompactionConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.JobLeaseDuration, prefix+"job-lease-duration", 15*time.Second, "")
	f.IntVar(&cfg.JobMaxFailures, prefix+"job-max-failures", 3, "")
}

var (
	// TODO aleks: for illustration purposes, to be moved externally
	globalCompactionStrategy = compactionStrategy{
		levels: map[uint32]compactionLevelStrategy{
			0: {maxBlocks: 20},
		},
		defaultStrategy: compactionLevelStrategy{
			maxBlocks: 10,
		},
		maxCompactionLevel: 3,
		// 0: 0.5
		// 1: 10s
		// 2: 100s
		// 3: 1000s // 16m40s
	}
)

type compactionStrategy struct {
	levels             map[uint32]compactionLevelStrategy
	defaultStrategy    compactionLevelStrategy
	maxCompactionLevel uint32
}

type compactionLevelStrategy struct {
	maxBlocks         int
	maxTotalSizeBytes uint64
}

func getStrategyForLevel(compactionLevel uint32) compactionLevelStrategy {
	strategy, ok := globalCompactionStrategy.levels[compactionLevel]
	if !ok {
		strategy = globalCompactionStrategy.defaultStrategy
	}
	return strategy
}

func (s compactionLevelStrategy) shouldCreateJob(blocks []string) bool {
	return len(blocks) >= s.maxBlocks
}

type compactionMetrics struct {
	addedBlocks   *prometheus.CounterVec
	deletedBlocks *prometheus.CounterVec
	addedJobs     *prometheus.CounterVec
	assignedJobs  *prometheus.CounterVec
	completedJobs *prometheus.CounterVec
	retriedJobs   *prometheus.CounterVec
	discardedJobs *prometheus.CounterVec
}

func newCompactionMetrics(reg prometheus.Registerer) *compactionMetrics {
	m := &compactionMetrics{
		addedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_added_blocks_count",
			Help:      "The number of blocks added for compaction",
		}, []string{"shard", "tenant", "level"}),
		deletedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_deleted_blocks_count",
			Help:      "The number of blocks deleted as a result of compaction",
		}, []string{"shard", "tenant", "level"}),
		addedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_added_jobs_count",
			Help:      "The number of created compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		assignedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_assigned_jobs_count",
			Help:      "The number of assigned compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		completedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_completed_jobs_count",
			Help:      "The number of completed compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		retriedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_retried_jobs_count",
			Help:      "The number of retried compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		discardedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope",
			Name:      "metastore_compaction_discarded_jobs_count",
			Help:      "The number of discarded compaction jobs",
		}, []string{"shard", "tenant", "level"}),
	}
	if reg != nil {
		util.Register(reg,
			m.addedBlocks,
			m.deletedBlocks,
			m.addedJobs,
			m.assignedJobs,
			m.completedJobs,
			m.retriedJobs,
			m.discardedJobs,
		)
	}
	return m
}
