package compactor

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
)

type Worker struct {
	service services.Service

	logger  log.Logger
	config  Config
	client  MetastoreClient
	storage objstore.Bucket
	metrics *metrics

	jobs     map[string]*compactionJob
	queue    chan *compactionJob
	threads  int
	capacity atomic.Int32

	stopped   atomic.Bool
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type Config struct {
	JobConcurrency  util.ConcurrencyLimit `yaml:"job_capacity"`
	JobPollInterval time.Duration         `yaml:"job_poll_interval"`
	SmallObjectSize int                   `yaml:"small_object_size_bytes"`
	TempDir         string                `yaml:"temp_dir"`
	RequestTimeout  time.Duration         `yaml:"request_timeout"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker."
	tempdir := filepath.Join(os.TempDir(), "pyroscope-compactor")
	f.TextVar(&cfg.JobConcurrency, prefix+"job-concurrency", util.GoMaxProcsConcurrencyLimit(), "Number of concurrent jobs compaction worker will run. Defaults to the number of CPU cores.")
	f.DurationVar(&cfg.JobPollInterval, prefix+"job-poll-interval", 5*time.Second, "Interval between job requests")
	f.DurationVar(&cfg.RequestTimeout, prefix+"request-timeout", 5*time.Second, "Job request timeout.")
	f.IntVar(&cfg.SmallObjectSize, prefix+"small-object-size-bytes", 8<<20, "Size of the object that can be loaded in memory.")
	f.StringVar(&cfg.TempDir, prefix+"temp-dir", tempdir, "Temporary directory for compaction jobs.")
}

type compactionJob struct {
	*metastorev1.CompactionJob

	ctx    context.Context
	cancel context.CancelFunc
	done   atomic.Bool

	blocks     []*metastorev1.BlockMeta
	assignment *metastorev1.CompactionJobAssignment
	compacted  *metastorev1.CompactedBlocks
}

type MetastoreClient interface {
	metastorev1.CompactionServiceClient
	metastorev1.IndexServiceClient
}

func New(
	logger log.Logger,
	config Config,
	client MetastoreClient,
	storage objstore.Bucket,
	reg prometheus.Registerer,
) (*Worker, error) {
	w := &Worker{
		config:  config,
		logger:  logger,
		client:  client,
		storage: storage,
		metrics: newMetrics(reg),
	}
	w.threads = int(config.JobConcurrency)
	w.queue = make(chan *compactionJob, 2*w.threads)
	w.jobs = make(map[string]*compactionJob, 2*w.threads)
	w.capacity.Store(int32(w.threads))
	w.service = services.NewBasicService(w.starting, w.running, w.stopping)
	return w, nil
}

func (w *Worker) Service() services.Service { return w.service }

func (w *Worker) starting(context.Context) (err error) { return nil }

func (w *Worker) stopping(error) error { return nil }

func (w *Worker) running(ctx context.Context) error {
	ticker := time.NewTicker(w.config.JobPollInterval)
	stopPolling := make(chan struct{})
	pollingDone := make(chan struct{})
	go func() {
		defer close(pollingDone)
		for {
			select {
			case <-stopPolling:
				return
			case <-ticker.C:
				w.poll()
			}
		}
	}()

	w.wg.Add(w.threads)
	for i := 0; i < w.threads; i++ {
		go func() {
			defer w.wg.Done()
			level.Info(w.logger).Log("msg", "compaction worker thread started")
			for job := range w.queue {
				w.capacity.Add(-1)
				util.Recover(func() { w.runCompaction(job) })
				job.done.Store(true)
				w.capacity.Add(1)
			}
		}()
	}

	<-ctx.Done()
	// Wait for all threads to finish their work, continuing to report status
	// updates about the in-progress jobs. First, signal to the poll loop that
	// we're done with new jobs.
	w.stopped.Store(true)
	level.Info(w.logger).Log("msg", "waiting for all jobs to finish")
	w.wg.Wait()

	// Now that all the threads are done, we stop the polling loop.
	ticker.Stop()
	close(stopPolling)
	<-pollingDone
	return nil
}

func (w *Worker) poll() {
	// Check if we want to stop polling for new jobs.
	// Close the queue if this is not the case.
	var capacity uint32
	if w.stopped.Load() {
		w.closeOnce.Do(func() {
			level.Info(w.logger).Log("msg", "closing job queue")
			close(w.queue)
		})
	} else {
		// We report the number of free workers in a hope to get more jobs.
		// Note that cap(w.queue) - len(w.queue) will only report 0 when all
		// the workers are busy and the queue is full (in fact, doubling the
		// reported capacity).
		if c := w.capacity.Load(); c > 0 {
			capacity = uint32(c)
		}
	}

	updates := w.collectUpdates()
	if len(updates) == 0 && capacity == 0 {
		level.Info(w.logger).Log("msg", "skipping polling", "updates", len(updates), "capacity", capacity)
		return
	}

	level.Info(w.logger).Log("msg", "polling compaction jobs", "updates", len(updates), "capacity", capacity)
	ctx, cancel := context.WithTimeout(context.Background(), w.config.RequestTimeout)
	defer cancel()
	resp, err := w.client.PollCompactionJobs(ctx, &metastorev1.PollCompactionJobsRequest{
		StatusUpdates: updates,
		JobCapacity:   capacity,
	})
	if err != nil {
		level.Error(w.logger).Log("msg", "failed to poll compaction jobs", "err", err)
		return
	}

	w.cleanup(updates)
	newJobs := w.handleResponse(resp)
	for _, job := range newJobs {
		select {
		case w.queue <- job:
		default:
			level.Warn(w.logger).Log("msg", "dropping job", "job_name", job.Name)
			w.remove(job)
		}
	}
}

func (w *Worker) collectUpdates() []*metastorev1.CompactionJobStatusUpdate {
	updates := make([]*metastorev1.CompactionJobStatusUpdate, 0, len(w.jobs))
	for _, job := range w.jobs {
		update := &metastorev1.CompactionJobStatusUpdate{
			Name:  job.Name,
			Token: job.assignment.Token,
		}

		switch done := job.done.Load(); {
		case done && job.compacted != nil:
			level.Info(w.logger).Log("msg", "sending update for completed job", "job", job.Name)
			update.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS
			update.CompactedBlocks = job.compacted
			updates = append(updates, update)

		case done && job.compacted == nil:
			// We're not sending a status update for the job and expect that the
			// assigment is to be revoked. The job is to be removed at the next
			// poll response handling: all jobs without assignments are canceled
			// and removed.
			level.Warn(w.logger).Log("msg", "skipping update for abandoned job", "job", job.Name)

		default:
			level.Info(w.logger).Log("msg", "sending update for in-progress job", "job", job.Name)
			update.Status = metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS
			updates = append(updates, update)
		}
	}

	return updates
}

func (w *Worker) cleanup(updates []*metastorev1.CompactionJobStatusUpdate) {
	for _, update := range updates {
		if job := w.jobs[update.Name]; job != nil && job.done.Load() {
			switch update.Status {
			case metastorev1.CompactionJobStatus_COMPACTION_STATUS_SUCCESS:
				// In the vast majority of cases, we end up here.
				w.remove(job)

			case metastorev1.CompactionJobStatus_COMPACTION_STATUS_IN_PROGRESS:
				// It is possible that the job has been completed after we
				// prepared the status update: keep the job for the next
				// poll iteration.

			default:
				// Workers never send other statuses. It's unexpected to get here.
				level.Warn(w.logger).Log("msg", "unexpected job status transition; removing the job", "job", job.Name)
				w.remove(job)
			}
		}
	}
}

func (w *Worker) remove(job *compactionJob) {
	delete(w.jobs, job.Name)
	job.cancel()
}

func (w *Worker) handleResponse(resp *metastorev1.PollCompactionJobsResponse) (newJobs []*compactionJob) {
	// Assignments by job name.
	assignments := make(map[string]*metastorev1.CompactionJobAssignment, len(resp.Assignments))
	for _, assignment := range resp.Assignments {
		assignments[assignment.Name] = assignment
	}

	for _, job := range w.jobs {
		if assignment, ok := assignments[job.assignment.Name]; ok {
			// In theory, we should respect the lease expiration time.
			// In practice, we have a static polling interval.
			job.assignment = assignment
		} else {
			// The job is running without an assigment.
			// We don't care how and when it ends.
			level.Warn(w.logger).Log("msg", "job re-assigned to another worker; cancelling", "job", job.Name)
			w.remove(job)
		}
	}

	for _, newJob := range resp.CompactionJobs {
		if running, found := w.jobs[newJob.Name]; found {
			level.Warn(w.logger).Log("msg", "job re-assigned to the same worker", "job", running.Name)
			// We're free to choose what to do. For now, we update the
			// assignment (in case the token has changed) and let the
			// running job finish.
			if running.assignment = assignments[running.Name]; running.assignment != nil {
				continue
			}
		}
		job := &compactionJob{CompactionJob: newJob}
		if job.assignment = assignments[newJob.Name]; job.assignment == nil {
			// That should not be possible, logging it here just in case.
			level.Warn(w.logger).Log("msg", "found a job without assigment", "job", job.Name)
			continue
		}
		job.ctx, job.cancel = context.WithCancel(context.Background())
		newJobs = append(newJobs, job)
		w.jobs[job.Name] = job
	}

	return newJobs
}

func (w *Worker) runCompaction(job *compactionJob) {
	start := time.Now()
	labels := []string{job.Tenant, strconv.Itoa(int(job.CompactionLevel))}
	statusName := "failure"
	defer func() {
		labelsWithStatus := append(labels, statusName)
		w.metrics.jobDuration.WithLabelValues(labelsWithStatus...).Observe(time.Since(start).Seconds())
		w.metrics.jobsCompleted.WithLabelValues(labelsWithStatus...).Inc()
		w.metrics.jobsInProgress.WithLabelValues(labels...).Dec()
	}()

	w.metrics.jobsInProgress.WithLabelValues(labels...).Inc()
	sp, ctx := opentracing.StartSpanFromContext(job.ctx, "runCompaction",
		opentracing.Tag{Key: "Job", Value: job.String()},
		opentracing.Tag{Key: "Tenant", Value: job.Tenant},
		opentracing.Tag{Key: "Shard", Value: job.Shard},
		opentracing.Tag{Key: "CompactionLevel", Value: job.CompactionLevel},
		opentracing.Tag{Key: "SourceBlocks", Value: len(job.SourceBlocks)},
		opentracing.Tag{Key: "Tombstones", Value: len(job.Tombstones)},
	)
	defer sp.Finish()

	logger := log.With(w.logger, "job", job.Name)
	deleteGroup, deleteCtx := errgroup.WithContext(ctx)
	for _, t := range job.Tombstones {
		if b := t.GetBlocks(); b != nil {
			deleteGroup.Go(func() error {
				// TODO(kolesnikovae): Clarify guarantees of cleanup.
				// Currently, we ignore any cleanup failures, as it's unlikely
				// that anyone would want to stop compaction due to a failed
				// cleanup. However, we should make this behavior configurable:
				// if cleanup fails, the entire job should be retried.
				w.deleteBlocks(deleteCtx, logger, b)
				return nil
			})
		}
	}

	level.Info(logger).Log(
		"msg", "starting compaction job",
		"source_blocks", strings.Join(job.SourceBlocks, " "),
	)
	if err := w.getBlockMetadata(logger, job); err != nil {
		return
	}

	tempdir := filepath.Join(w.config.TempDir, job.Name)
	sourcedir := filepath.Join(tempdir, "source")
	compacted, err := block.Compact(ctx, job.blocks, w.storage,
		block.WithCompactionTempDir(tempdir),
		block.WithCompactionObjectOptions(
			block.WithObjectMaxSizeLoadInMemory(w.config.SmallObjectSize),
			block.WithObjectDownload(sourcedir),
		))

	switch {
	case err == nil:
		level.Info(logger).Log(
			"msg", "compaction finished successfully",
			"input_blocks", len(job.SourceBlocks),
			"output_blocks", len(compacted),
		)
		for _, c := range compacted {
			level.Info(logger).Log(
				"msg", "new compacted block",
				"block_id", block.ID(c),
				"block_tenant", c.StringTable[c.Tenant],
				"block_shard", c.Shard,
				"block_compaction_level", c.CompactionLevel,
				"block_min_time", c.MinTime,
				"block_max_time", c.MinTime,
				"block_size", c.Size,
				"datasets", len(c.Datasets),
			)
		}

		statusName = "success"
		job.compacted = &metastorev1.CompactedBlocks{
			NewBlocks: compacted,
			SourceBlocks: &metastorev1.BlockList{
				Tenant: job.Tenant,
				Shard:  job.Shard,
				Blocks: job.SourceBlocks,
			},
		}

		firstBlock := block.Timestamp(job.blocks[0])
		w.metrics.timeToCompaction.WithLabelValues(labels...).Observe(time.Since(firstBlock).Seconds())

	case errors.Is(err, context.Canceled):
		level.Warn(logger).Log("msg", "job cancelled")
		statusName = "cancelled"

	default:
		level.Error(logger).Log("msg", "failed to compact blocks", "err", err)
	}

	// The only error returned by Wait is the context
	// cancellation error handled above.
	_ = deleteGroup.Wait()
}

func (w *Worker) getBlockMetadata(logger log.Logger, job *compactionJob) error {
	ctx, cancel := context.WithTimeout(job.ctx, w.config.RequestTimeout)
	defer cancel()

	resp, err := w.client.GetBlockMetadata(ctx, &metastorev1.GetBlockMetadataRequest{
		Blocks: &metastorev1.BlockList{
			Tenant: job.Tenant,
			Shard:  job.Shard,
			Blocks: job.SourceBlocks,
		},
	})
	if err != nil {
		level.Error(logger).Log("msg", "failed to get block metadata", "err", err)
		return err
	}

	source := resp.GetBlocks()
	if len(source) == 0 {
		level.Warn(logger).Log(
			"msg", "no block metadata found; skipping",
			"blocks", len(job.SourceBlocks),
			"blocks_found", len(source),
		)
		return fmt.Errorf("no blocks to compact")
	}

	// Update the plan to reflect the actual compaction job state.
	job.SourceBlocks = job.SourceBlocks[:0]
	for _, b := range source {
		job.SourceBlocks = append(job.SourceBlocks, b.StringTable[b.Id])
	}

	job.blocks = source
	return nil
}

func (w *Worker) deleteBlocks(ctx context.Context, logger log.Logger, t *metastorev1.BlockTombstones) {
	level.Info(logger).Log(
		"msg", "deleting blocks",
		"tenant", t.Tenant,
		"shard", t.Shard,
		"compaction_level", t.CompactionLevel,
		"blocks", strings.Join(t.Blocks, " "),
	)
	for _, b := range t.Blocks {
		path := block.BuildObjectPath(t.Tenant, t.Shard, t.CompactionLevel, b)
		if err := w.storage.Delete(ctx, path); err != nil {
			if objstore.IsNotExist(w.storage, err) {
				level.Warn(logger).Log("msg", "block not found", "path", path, "err", err)
				continue
			}
			level.Warn(logger).Log("msg", "failed to delete block", "path", path, "err", err)
		}
	}
}
