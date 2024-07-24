package compactionworker

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/metastore/client"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/querybackend/block"
)

type Worker struct {
	*services.BasicService

	config          Config
	logger          log.Logger
	metastoreClient *metastoreclient.Client
	storage         objstore.Bucket
	metrics         *compactionWorkerMetrics

	jobMutex             sync.RWMutex
	pendingJobs          map[string]*compactorv1.CompactionJob
	activeJobs           map[string]*compactorv1.CompactionJob
	pendingStatusUpdates map[string]*compactorv1.CompactionJobStatus
}

type Config struct {
	JobCapacity int `yaml:"job_capacity"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker."
	f.IntVar(&cfg.JobCapacity, prefix+"job-capacity", 5, "how many concurrent jobs will a worker run at most")
}

func New(config Config, logger log.Logger, metastoreClient *metastoreclient.Client, storage objstore.Bucket, reg prometheus.Registerer) (*Worker, error) {
	w := &Worker{
		config:               config,
		logger:               logger,
		metastoreClient:      metastoreClient,
		storage:              storage,
		pendingJobs:          make(map[string]*compactorv1.CompactionJob),
		activeJobs:           make(map[string]*compactorv1.CompactionJob),
		pendingStatusUpdates: make(map[string]*compactorv1.CompactionJobStatus),
		metrics:              newMetrics(reg),
	}
	w.BasicService = services.NewBasicService(w.starting, w.running, w.stopping)
	return w, nil
}

func (w *Worker) starting(ctx context.Context) (err error) {
	return nil
}

func (w *Worker) running(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.poll(ctx)

			w.jobMutex.RLock()
			pendingJobs := make(map[string]*compactorv1.CompactionJob, len(w.pendingJobs))
			for _, job := range w.pendingJobs {
				pendingJobs[job.Name] = job
			}
			w.jobMutex.RUnlock()

			if len(pendingJobs) > 0 {
				level.Info(w.logger).Log("msg", "starting pending compaction jobs", "pendingJobs", len(pendingJobs))
				for _, job := range pendingJobs {
					job := job
					go func() {
						w.jobMutex.Lock()
						w.activeJobs[job.Name] = job
						delete(w.pendingJobs, job.Name)
						w.jobMutex.Unlock()

						level.Info(w.logger).Log("msg", "starting compaction job", "job", job.Name)
						status := w.startJob(ctx, job)

						level.Info(w.logger).Log("msg", "compaction job finished", "job", job.Name)

						w.jobMutex.Lock()
						w.pendingStatusUpdates[job.Name] = status
						delete(w.activeJobs, job.Name)
						w.jobMutex.Unlock()
					}()
				}
			}

		case <-ctx.Done():
			return nil
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	w.jobMutex.Lock()
	level.Debug(w.logger).Log(
		"msg", "polling for compaction jobs and status updates",
		"active_jobs", len(w.activeJobs),
		"pending_jobs", len(w.pendingJobs),
		"pending_updates", len(w.pendingStatusUpdates))

	pendingStatusUpdates := make([]*compactorv1.CompactionJobStatus, 0, len(w.pendingStatusUpdates))
	for _, update := range w.pendingStatusUpdates {
		level.Debug(w.logger).Log("msg", "pending compaction job update", "job", update.JobName, "status", update.Status)
		pendingStatusUpdates = append(pendingStatusUpdates, update)
	}
	for _, activeJob := range w.activeJobs {
		level.Debug(w.logger).Log("msg", "in progress job update", "job", activeJob.Name)
		pendingStatusUpdates = append(pendingStatusUpdates, &compactorv1.CompactionJobStatus{
			JobName:      activeJob.Name,
			Status:       compactorv1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS,
			RaftLogIndex: activeJob.RaftLogIndex,
			Shard:        activeJob.Shard,
			TenantId:     activeJob.TenantId,
		})
	}
	jobCapacity := uint32(w.config.JobCapacity - len(w.activeJobs) - len(w.pendingJobs))
	w.jobMutex.Unlock()

	if len(pendingStatusUpdates) > 0 || jobCapacity > 0 {
		jobsResponse, err := w.metastoreClient.PollCompactionJobs(ctx, &compactorv1.PollCompactionJobsRequest{
			JobStatusUpdates: pendingStatusUpdates,
			JobCapacity:      jobCapacity,
		})

		if err != nil {
			level.Error(w.logger).Log("msg", "failed to poll compaction jobs", "err", err)
			return
		}

		level.Debug(w.logger).Log("msg", "poll response received", "compaction_jobs", len(jobsResponse.CompactionJobs))

		w.jobMutex.Lock()
		for _, update := range pendingStatusUpdates {
			delete(w.pendingStatusUpdates, update.JobName)
		}

		for _, pendingJob := range jobsResponse.CompactionJobs {
			w.pendingJobs[pendingJob.Name] = pendingJob
		}
		w.jobMutex.Unlock()
	}
}

func (w *Worker) stopping(err error) error {
	// TODO aleks: handle shutdown
	return nil
}

func (w *Worker) startJob(ctx context.Context, job *compactorv1.CompactionJob) *compactorv1.CompactionJobStatus {
	jobStartTime := time.Now()
	labels := []string{job.TenantId, fmt.Sprint(job.Shard), fmt.Sprint(job.CompactionLevel)}
	jobOutcome := "unknown"
	defer func() {
		elapsed := time.Since(jobStartTime)
		labelsWithOutcome := append(labels, jobOutcome)
		w.metrics.jobsCompleted.WithLabelValues(labelsWithOutcome...).Inc()
		w.metrics.jobsInProgress.WithLabelValues(labels...).Dec()
		w.metrics.jobDuration.WithLabelValues(labelsWithOutcome...).Observe(elapsed.Seconds())
	}()
	w.metrics.jobsInProgress.WithLabelValues(labels...).Inc()

	sp, ctx := opentracing.StartSpanFromContext(ctx, "StartCompactionJob",
		opentracing.Tag{Key: "Job", Value: job.String()},
		opentracing.Tag{Key: "Tenant", Value: job.TenantId},
		opentracing.Tag{Key: "Shard", Value: job.Shard},
		opentracing.Tag{Key: "CompactionLevel", Value: job.CompactionLevel},
		opentracing.Tag{Key: "BlockCount", Value: len(job.Blocks)},
	)
	defer sp.Finish()

	jobStatus := &compactorv1.CompactionJobStatus{
		JobName:      job.Name,
		CompletedJob: &compactorv1.CompletedJob{},
		Shard:        job.Shard,
		TenantId:     job.TenantId,
		RaftLogIndex: job.RaftLogIndex,
	}

	level.Info(w.logger).Log(
		"msg", "compacting blocks for job",
		"job", job.Name,
		"blocks", len(job.Blocks))

	compactedBlockMetas, err := block.Compact(ctx, job.Blocks, w.storage, block.WithObjectMaxSizeLoadInMemory(4<<20))
	if err != nil {
		level.Error(w.logger).Log("msg", "failed to run block compaction", "err", err, "job", job.Name)
		jobStatus.Status = compactorv1.CompactionStatus_COMPACTION_STATUS_FAILURE
		jobOutcome = "failure"
		return jobStatus
	}

	level.Info(w.logger).Log(
		"msg", "successful compaction for job",
		"job", job.Name,
		"blocks", len(job.Blocks),
		"output_blocks", len(compactedBlockMetas))

	jobStatus.Status = compactorv1.CompactionStatus_COMPACTION_STATUS_SUCCESS
	jobOutcome = "success"
	jobStatus.CompletedJob.Blocks = compactedBlockMetas

	return jobStatus
}
