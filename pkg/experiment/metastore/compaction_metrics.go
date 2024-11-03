package metastore

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/util"
)

type compactionMetrics struct {
	addedBlocks   *prometheus.CounterVec
	deletedBlocks *prometheus.CounterVec
	addedJobs     *prometheus.CounterVec
	assignedJobs  *prometheus.CounterVec
	completedJobs *prometheus.CounterVec
	retriedJobs   *prometheus.CounterVec
	discardedJobs *prometheus.CounterVec
	invalidJobs   *prometheus.CounterVec
}

func newCompactionMetrics(reg prometheus.Registerer) *compactionMetrics {
	m := &compactionMetrics{
		addedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_added_blocks_count",
			Help: "The number of blocks added for compaction",
		}, []string{"shard", "tenant", "level"}),
		deletedBlocks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_deleted_blocks_count",
			Help: "The number of blocks deleted as a result of compaction",
		}, []string{"shard", "tenant", "level"}),
		addedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_added_jobs_count",
			Help: "The number of created compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		assignedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_assigned_jobs_count",
			Help: "The number of assigned compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		completedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_completed_jobs_count",
			Help: "The number of completed compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		retriedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_retried_jobs_count",
			Help: "The number of retried compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		discardedJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_discarded_jobs_count",
			Help: "The number of discarded compaction jobs",
		}, []string{"shard", "tenant", "level"}),
		invalidJobs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "metastore_compaction_invalid_jobs_count",
			Help: "The number of invalid compaction jobs",
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
			m.invalidJobs,
		)
	}
	return m
}

func compactionMetricDimsBlock(md *metastorev1.BlockMeta) []string {
	return []string{
		strconv.Itoa(int(md.Shard)),
		md.TenantId,
		strconv.Itoa(int(md.CompactionLevel)),
	}
}

func compactionMetricDimsJob(md *raft_log.CompactionJob) []string {
	return []string{
		strconv.Itoa(int(md.Shard)),
		md.Tenant,
		strconv.Itoa(int(md.CompactionLevel)),
	}
}
