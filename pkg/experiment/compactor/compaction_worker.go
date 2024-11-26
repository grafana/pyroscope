package compactor

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	labels2 "github.com/prometheus/prometheus/model/labels"
	_ "go.uber.org/automaxprocs"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	"github.com/grafana/pyroscope/pkg/experiment/query_backend"
	"github.com/grafana/pyroscope/pkg/experiment/query_backend/block"
	"github.com/grafana/pyroscope/pkg/objstore"
)

type Worker struct {
	*services.BasicService

	config          Config
	logger          log.Logger
	metastoreClient *metastoreclient.Client
	storage         objstore.Bucket
	metrics         *compactionWorkerMetrics

	jobMutex      sync.RWMutex
	pendingJobs   map[string]*metastorev1.CompactionJob
	activeJobs    map[string]*metastorev1.CompactionJob
	completedJobs map[string]*metastorev1.CompactionJobStatus

	queue chan *metastorev1.CompactionJob
	wg    sync.WaitGroup

	exporter *Exporter
}

type Config struct {
	JobConcurrency  int           `yaml:"job_capacity"`
	JobPollInterval time.Duration `yaml:"job_poll_interval"`
	SmallObjectSize int           `yaml:"small_object_size_bytes"`
	TempDir         string        `yaml:"temp_dir"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker."
	tempdir := filepath.Join(os.TempDir(), "pyroscope-compactor")
	f.IntVar(&cfg.JobConcurrency, prefix+"job-concurrency", 1, "How many concurrent jobs will a compaction worker run at most.")
	f.DurationVar(&cfg.JobPollInterval, prefix+"job-poll-interval", 5*time.Second, "How often will a compaction worker poll for jobs.")
	f.IntVar(&cfg.SmallObjectSize, prefix+"small-object-size-bytes", 8<<20, "Size of the object that can be loaded in memory.")
	f.StringVar(&cfg.TempDir, prefix+"temp-dir", tempdir, "Temporary directory for compaction jobs.")
}

func (cfg *Config) Validate() error {
	// TODO(kolesnikovae): implement.
	return nil
}

func New(config Config, logger log.Logger, metastoreClient *metastoreclient.Client, storage objstore.Bucket, reg prometheus.Registerer) (*Worker, error) {
	workers := runtime.GOMAXPROCS(-1) * config.JobConcurrency
	w := &Worker{
		config:          config,
		logger:          logger,
		metastoreClient: metastoreClient,
		storage:         storage,
		pendingJobs:     make(map[string]*metastorev1.CompactionJob),
		activeJobs:      make(map[string]*metastorev1.CompactionJob),
		completedJobs:   make(map[string]*metastorev1.CompactionJobStatus),
		metrics:         newMetrics(reg),
		queue:           make(chan *metastorev1.CompactionJob, workers),
		exporter:        NewExporter(),
	}
	w.BasicService = services.NewBasicService(w.starting, w.running, w.stopping)
	return w, nil
}

func (w *Worker) starting(ctx context.Context) (err error) {
	return nil
}

func (w *Worker) running(ctx context.Context) error {
	ticker := time.NewTicker(w.config.JobPollInterval)
	defer ticker.Stop()
	for i := 0; i < cap(w.queue); i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.jobsLoop(ctx)
		}()
	}

	for {
		select {
		case <-ticker.C:
			w.poll(ctx)

		case <-ctx.Done():
			w.wg.Wait()
			return nil
		}
	}
}

func (w *Worker) jobsLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case job := <-w.queue:
			w.jobMutex.Lock()
			delete(w.pendingJobs, job.Name)
			w.activeJobs[job.Name] = job
			w.jobMutex.Unlock()

			_ = level.Info(w.logger).Log("msg", "starting compaction job", "job", job.Name)
			status := w.startJob(ctx, job)
			_ = level.Info(w.logger).Log("msg", "compaction job finished", "job", job.Name)

			w.jobMutex.Lock()
			delete(w.activeJobs, job.Name)
			w.completedJobs[job.Name] = status
			w.jobMutex.Unlock()
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	w.jobMutex.Lock()
	level.Debug(w.logger).Log(
		"msg", "polling for compaction jobs and status updates",
		"active_jobs", len(w.activeJobs),
		"pending_jobs", len(w.pendingJobs),
		"pending_updates", len(w.completedJobs))

	pendingStatusUpdates := make([]*metastorev1.CompactionJobStatus, 0, len(w.completedJobs))
	for _, update := range w.completedJobs {
		level.Debug(w.logger).Log("msg", "completed job update", "job", update.JobName, "status", update.Status)
		pendingStatusUpdates = append(pendingStatusUpdates, update)
	}
	for _, activeJob := range w.activeJobs {
		level.Debug(w.logger).Log("msg", "in progress job update", "job", activeJob.Name)
		update := activeJob.Status.CloneVT()
		update.Status = metastorev1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS
		pendingStatusUpdates = append(pendingStatusUpdates, update)
	}
	for _, pendingJob := range w.pendingJobs {
		level.Debug(w.logger).Log("msg", "pending job update", "job", pendingJob.Name)
		update := pendingJob.Status.CloneVT()
		update.Status = metastorev1.CompactionStatus_COMPACTION_STATUS_IN_PROGRESS
		pendingStatusUpdates = append(pendingStatusUpdates, update)
	}

	jobCapacity := cap(w.queue) - len(w.queue)
	w.jobMutex.Unlock()

	if len(pendingStatusUpdates) > 0 || jobCapacity > 0 {
		jobsResponse, err := w.metastoreClient.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{
			JobStatusUpdates: pendingStatusUpdates,
			JobCapacity:      uint32(jobCapacity),
		})

		if err != nil {
			level.Error(w.logger).Log("msg", "failed to poll compaction jobs", "err", err)
			return
		}

		level.Debug(w.logger).Log("msg", "poll response received", "compaction_jobs", len(jobsResponse.CompactionJobs))

		pendingJobs := make([]*metastorev1.CompactionJob, 0, len(jobsResponse.CompactionJobs))
		for _, job := range jobsResponse.CompactionJobs {
			pendingJobs = append(pendingJobs, job.CloneVT())
		}

		w.jobMutex.Lock()
		for _, update := range pendingStatusUpdates {
			delete(w.completedJobs, update.JobName)
		}
		for _, job := range pendingJobs {
			w.pendingJobs[job.Name] = job
		}
		w.jobMutex.Unlock()

		for _, job := range pendingJobs {
			select {
			case w.queue <- job:
			default:
				level.Warn(w.logger).Log("msg", "dropping job", "job_name", job.Name)
				w.jobMutex.Lock()
				delete(w.pendingJobs, job.Name)
				w.jobMutex.Unlock()
			}
		}
	}
}

func (w *Worker) stopping(err error) error {
	// TODO aleks: handle shutdown
	return nil
}

func (w *Worker) startJob(ctx context.Context, job *metastorev1.CompactionJob) *metastorev1.CompactionJobStatus {
	jobStartTime := time.Now()
	labels := []string{job.TenantId, fmt.Sprint(job.Shard), fmt.Sprint(job.CompactionLevel)}
	statusName := "unknown"
	defer func() {
		elapsed := time.Since(jobStartTime)
		jobStatusLabel := append(labels, statusName)
		w.metrics.jobDuration.WithLabelValues(jobStatusLabel...).Observe(elapsed.Seconds())
		w.metrics.jobsCompleted.WithLabelValues(jobStatusLabel...).Inc()
		w.metrics.jobsInProgress.WithLabelValues(labels...).Dec()
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

	_ = level.Info(w.logger).Log(
		"msg", "compacting blocks for job",
		"job", job.Name,
		"blocks", len(job.Blocks))

	tempdir := filepath.Join(w.config.TempDir, job.Name)
	sourcedir := filepath.Join(tempdir, "source")
	// TODO(kolesnikovae): Return the actual error once we
	//   can handle compaction failures in metastore.
	compacted, err := pretendEverythingIsOK(func() ([]*metastorev1.BlockMeta, error) {
		return block.Compact(ctx, job.Blocks, w.storage,
			block.WithCompactionTempDir(tempdir),
			block.WithCompactionObjectOptions(
				block.WithObjectMaxSizeLoadInMemory(w.config.SmallObjectSize),
				block.WithObjectDownload(sourcedir),
			),
		)
	})

	logger := log.With(w.logger,
		"job_name", job.Name,
		"job_shard", job.Shard,
		"job_tenant", job.TenantId,
		"job_compaction_level", job.CompactionLevel,
	)

	switch {
	case err == nil:
		_ = level.Info(logger).Log(
			"msg", "successful compaction for job",
			"input_blocks", len(job.Blocks),
			"output_blocks", len(compacted))

		for _, c := range compacted {
			_ = level.Info(logger).Log(
				"msg", "new compacted block",
				"block_id", c.Id,
				"block_tenant", c.TenantId,
				"block_shard", c.Shard,
				"block_size", c.Size,
				"block_compaction_level", c.CompactionLevel,
				"block_min_time", c.MinTime,
				"block_max_time", c.MinTime,
				"datasets", len(c.Datasets))
		}

		if job.CompactionLevel == 0 {
			w.exportMetricsFromCompactedBlocks(ctx, compacted)
		}

		job.Status.Status = metastorev1.CompactionStatus_COMPACTION_STATUS_SUCCESS
		job.Status.CompletedJob = &metastorev1.CompletedJob{Blocks: compacted}
		statusName = "success"

	case errors.Is(err, context.Canceled):
		_ = level.Warn(logger).Log("msg", "job cancelled", "job", job.Name)
		job.Status.Status = metastorev1.CompactionStatus_COMPACTION_STATUS_UNSPECIFIED
		statusName = "cancelled"

	default:
		_ = level.Error(logger).Log("msg", "failed to compact blocks", "err", err, "job", job.Name)
		job.Status.Status = metastorev1.CompactionStatus_COMPACTION_STATUS_FAILURE
		statusName = "failure"
	}

	return job.Status
}

func (w *Worker) exportMetricsFromCompactedBlocks(ctx context.Context, compacted []*metastorev1.BlockMeta) {
	functions := map[string]*queryv1.FunctionList{
		// TODO:
		// This must be richer. First, it should be split by tenant.
		// Also, we could have functions associated to service_name
		// while others are just collected generally no matter what
		// service_name we handle
		"pyroscope": {
			Functions: []string{
				"net/http.HandlerFunc.ServeHTTP",
				"runtime.gcBgMarkWorker",
			},
		},
		"ride-sharing-app": {
			Functions: []string{
				"net/http.HandlerFunc.ServeHTTP",
				"runtime.gcBgMarkWorker",
			},
		},
	}
	for _, c := range compacted {
		reader := query_backend.NewBlockReader(w.logger, w.storage)
		var res, _ = reader.Invoke(ctx,
			&queryv1.InvokeRequest{
				Tenant:    []string{c.TenantId},
				StartTime: c.MinTime,
				EndTime:   c.MaxTime,
				Query: []*queryv1.Query{{
					QueryType: queryv1.QueryType_QUERY_METRICS,
					Metrics: &queryv1.MetricsQuery{
						FunctionsByServiceName: functions,
					},
				}},
				QueryPlan: &queryv1.QueryPlan{
					Root: &queryv1.QueryNode{
						Blocks: []*metastorev1.BlockMeta{c},
					},
				},
				LabelSelector: "{}",
			},
		)

		// convert metrics into the expected export format
		wr := WriteRequest{}
		for _, series := range res.Reports[0].Metrics.GetTimeSeries() {
			timeSeries := TimeSeries{}
			for _, label := range series.Labels {
				timeSeries.Labels = append(timeSeries.Labels, labels2.Label{
					Name:  label.Name,
					Value: label.Value,
				})
			}
			for _, point := range series.Points {
				timeSeries.Samples = append(timeSeries.Samples, Sample{
					Value:     point.Value,
					Timestamp: point.Timestamp,
				})
			}
			wr.TimeSeries = append(wr.TimeSeries, timeSeries)
		}
		if len(wr.TimeSeries) > 0 {
			go func() {
				if sendErr := w.exporter.Send(context.Background(), &wr); sendErr != nil {
					_ = w.logger.Log("msg", "failed to push metrics", "err", sendErr)
				}
			}()
		}
	}
}

func pretendEverythingIsOK(fn func() ([]*metastorev1.BlockMeta, error)) (m []*metastorev1.BlockMeta, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("ignoring compaction panic:", r)
			fmt.Println(string(debug.Stack()))
			m = nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// We can handle this.
				return
			}
			fmt.Println("ignoring compaction error:", err)
			m = nil
		}
		err = nil
	}()
	return fn()
}
