package compactionworker

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid/v2"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	thanosstore "github.com/thanos-io/objstore"
	_ "go.uber.org/automaxprocs"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/metrics"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
)

type Config struct {
	JobConcurrency     int            `yaml:"job_capacity"`
	JobPollInterval    time.Duration  `yaml:"job_poll_interval"`
	SmallObjectSize    int            `yaml:"small_object_size_bytes"`
	TempDir            string         `yaml:"temp_dir"`
	RequestTimeout     time.Duration  `yaml:"request_timeout"`
	CleanupMaxDuration time.Duration  `yaml:"cleanup_max_duration"`
	MetricsExporter    metrics.Config `yaml:"metrics_exporter"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	const prefix = "compaction-worker."
	f.IntVar(&cfg.JobConcurrency, prefix+"job-concurrency", 0, "Number of concurrent jobs compaction worker will run. Defaults to the number of CPU cores.")
	f.DurationVar(&cfg.JobPollInterval, prefix+"job-poll-interval", 5*time.Second, "Interval between job requests")
	f.DurationVar(&cfg.RequestTimeout, prefix+"request-timeout", 5*time.Second, "Job request timeout.")
	f.DurationVar(&cfg.CleanupMaxDuration, prefix+"cleanup-max-duration", 15*time.Second, "Maximum duration of the cleanup operations.")
	f.IntVar(&cfg.SmallObjectSize, prefix+"small-object-size-bytes", 8<<20, "Size of the object that can be loaded in memory.")
	f.StringVar(&cfg.TempDir, prefix+"temp-dir", os.TempDir(), "Temporary directory for compaction jobs.")
	cfg.MetricsExporter.RegisterFlags(f)
}

type Worker struct {
	service services.Service

	logger    log.Logger
	config    Config
	client    MetastoreClient
	storage   objstore.Bucket
	compactFn compactFunc
	metrics   *workerMetrics

	jobs     map[string]*compactionJob
	queue    chan *compactionJob
	threads  int
	capacity atomic.Int32

	deleterPool *deleterPool

	stopped   atomic.Bool
	closeOnce sync.Once
	wg        sync.WaitGroup

	exporter metrics.Exporter
	ruler    metrics.Ruler
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

type compactFunc func(context.Context, []*metastorev1.BlockMeta, objstore.Bucket, ...block.CompactionOption) ([]*metastorev1.BlockMeta, error)

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
	ruler metrics.Ruler,
	exporter metrics.Exporter,
) (*Worker, error) {
	config.TempDir = filepath.Join(filepath.Clean(config.TempDir), "pyroscope-compactor")
	_ = os.RemoveAll(config.TempDir)
	if err := os.MkdirAll(config.TempDir, 0o777); err != nil {
		return nil, fmt.Errorf("failed to create compactor directory: %w", err)
	}
	w := &Worker{
		config:    config,
		logger:    logger,
		client:    client,
		storage:   storage,
		compactFn: block.Compact,
		metrics:   newMetrics(reg),
		ruler:     ruler,
		exporter:  exporter,
	}
	w.threads = config.JobConcurrency
	if w.threads < 1 {
		w.threads = runtime.GOMAXPROCS(-1)
	}
	w.queue = make(chan *compactionJob, 2*w.threads)
	w.jobs = make(map[string]*compactionJob, 2*w.threads)
	w.capacity.Store(int32(w.threads))
	w.deleterPool = newDeleterPool(16 * w.threads)
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
				// Now that all the threads are done, we need to
				// send the final status updates.
				w.poll()
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
	// Force exporter to send all staged samples (depends on the implementation)
	// Must be a blocking call.
	if w.exporter != nil {
		w.exporter.Flush()
	}
	w.deleterPool.close()
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
			// We're not sending the status update for the job and expect that the
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

// Status is only used in metrics and logging.
type status string

const (
	statusSuccess          status = "success"
	statusFailure          status = "failure"
	statusCanceled         status = "canceled"
	statusMetadataNotFound status = "metadata_not_found"
	statusBlockNotFound    status = "block_not_found"
)

func (w *Worker) runCompaction(job *compactionJob) {
	start := time.Now()
	metricLabels := []string{job.Tenant, strconv.Itoa(int(job.CompactionLevel))}
	statusName := statusFailure
	defer func() {
		labelsWithStatus := append(metricLabels, string(statusName))
		w.metrics.jobDuration.WithLabelValues(labelsWithStatus...).Observe(time.Since(start).Seconds())
		w.metrics.jobsCompleted.WithLabelValues(labelsWithStatus...).Inc()
		w.metrics.jobsInProgress.WithLabelValues(metricLabels...).Dec()
	}()

	w.metrics.jobsInProgress.WithLabelValues(metricLabels...).Inc()
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
	level.Info(logger).Log("msg", "starting compaction job", "source_blocks", strings.Join(job.SourceBlocks, " "))

	// FIXME(kolesnikovae): Read metadata from blocks: it's located in the
	//   blocks footer. The start offest and CRC are the last 8 bytes (BE).
	//   See metadata.Encode and metadata.Decode.
	//   We use metadata to download objects: in fact we need to know only
	//   tenant, shard, level, and ID: the information which we already have
	//   in the job. We definitely don't need the full metadata entry with
	//   datasets: this part can be set once we download the block and read
	//   meta locally. Or, we can just fetch the metadata from the objects
	//   directly, before downloading them.
	if err := w.getBlockMetadata(logger, job); err != nil {
		// The error is likely to be transient, therefore the job is not failed,
		// but just abandoned – another worker will pick it up and try again.
		return
	}

	if len(job.Tombstones) > 0 {
		// Handle tombstones asynchronously on the best effort basis:
		// if deletion fails, leftovers will be cleaned up eventually.
		//
		// There are following reasons why we may not be able to delete:
		//  1. General storage unavailability: compaction jobs will be
		//     retried either way, and the tombstones will be handled again.
		//  2. Permission issues. In this case, retry will not help.
		//  3. Worker crash: jobs will be retried.
		//
		// A worker is given a limited time to finish the cleanup. If worker
		// didn't finish the cleanup before shutdown and after the compaction
		// job was finished (so no retry is expected), the data will be deleted
		// eventually due to time-based retention policy. However, if no more
		// tombstones are created for the shard, the data will remain in the
		// storage. This should be handled by the index cleaner: some garbage
		// collection should happen in the background.
		w.handleTombstones(logger, job.Tombstones...)
	}

	if len(job.blocks) == 0 {
		// This is a very bad situation that we do not expect, unless the
		// metastore is restored from a snapshot: no metadata found for the
		// job source blocks. There's no point in retrying or failing the
		// job (which is likely to be retried by another worker), so we just
		// skip it. The same for the situation when no block objects can be
		// found in storage, which may happen if the blocks are deleted manually.
		level.Error(logger).Log("msg", "no block metadata found; skipping")
		job.compacted = &metastorev1.CompactedBlocks{SourceBlocks: new(metastorev1.BlockList)}
		statusName = statusMetadataNotFound
		return
	}

	tempdir := filepath.Join(w.config.TempDir, job.Name)
	sourcedir := filepath.Join(tempdir, "source")
	options := []block.CompactionOption{
		block.WithCompactionTempDir(tempdir),
		block.WithCompactionObjectOptions(
			block.WithObjectMaxSizeLoadInMemory(w.config.SmallObjectSize),
			block.WithObjectDownload(sourcedir),
		),
	}

	if observer := w.buildSampleObserver(job.blocks[0]); observer != nil {
		defer observer.Close()
		options = append(options, block.WithSampleObserver(observer))
	}

	compacted, err := w.compactFn(ctx, job.blocks, w.storage, options...)
	defer func() {
		if err = os.RemoveAll(tempdir); err != nil {
			level.Warn(logger).Log("msg", "failed to remove compaction directory", "path", tempdir, "err", err)
		}
	}()

	switch {
	case err == nil:
		level.Info(logger).Log(
			"msg", "compaction finished successfully",
			"input_blocks", len(job.SourceBlocks),
			"output_blocks", len(compacted),
		)
		for _, c := range compacted {
			level.Debug(logger).Log(
				"msg", "new compacted block",
				"block_id", c.Id,
				"block_tenant", metadata.Tenant(c),
				"block_shard", c.Shard,
				"block_compaction_level", c.CompactionLevel,
				"block_min_time", c.MinTime,
				"block_max_time", c.MaxTime,
				"block_size", c.Size,
				"datasets", len(c.Datasets),
			)
		}
		job.compacted = &metastorev1.CompactedBlocks{
			NewBlocks: compacted,
			SourceBlocks: &metastorev1.BlockList{
				Tenant: job.Tenant,
				Shard:  job.Shard,
				Blocks: job.SourceBlocks,
			},
		}

		firstBlock := metadata.Timestamp(job.blocks[0])
		w.metrics.timeToCompaction.WithLabelValues(metricLabels...).Observe(time.Since(firstBlock).Seconds())
		statusName = statusSuccess

	case errors.Is(err, context.Canceled):
		level.Warn(logger).Log("msg", "compaction cancelled")
		statusName = statusCanceled

	case objstore.IsNotExist(w.storage, err):
		level.Error(logger).Log("msg", "failed to find blocks", "err", err)
		job.compacted = &metastorev1.CompactedBlocks{SourceBlocks: new(metastorev1.BlockList)}
		statusName = statusBlockNotFound

	default:
		level.Error(logger).Log("msg", "failed to compact blocks", "err", err)
		statusName = statusFailure
	}
}

func (w *Worker) buildSampleObserver(md *metastorev1.BlockMeta) *metrics.SampleObserver {
	if !w.config.MetricsExporter.Enabled || md.CompactionLevel > 0 {
		return nil
	}
	recordingTime := int64(ulid.MustParse(md.Id).Time())
	pyroscopeInstanceLabel := labels.New(labels.Label{
		Name:  "pyroscope_instance",
		Value: pyroscopeInstanceHash(md.Shard, uint32(md.CreatedBy)),
	})
	return metrics.NewSampleObserver(recordingTime, w.exporter, w.ruler, pyroscopeInstanceLabel)
}

func pyroscopeInstanceHash(shard uint32, createdBy uint32) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[0:4], shard)
	binary.BigEndian.PutUint32(buf[4:8], createdBy)
	return fmt.Sprintf("%x", xxhash.Sum64(buf))
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

	job.blocks = resp.GetBlocks()
	// Update the plan to reflect the actual compaction job state.
	job.SourceBlocks = job.SourceBlocks[:0]
	for _, b := range job.blocks {
		job.SourceBlocks = append(job.SourceBlocks, b.Id)
	}

	return nil
}

func (w *Worker) handleTombstones(logger log.Logger, tombstones ...*metastorev1.Tombstones) {
	for _, t := range tombstones {
		w.deleterPool.add(w.newDeleter(logger, t), w.config.CleanupMaxDuration)
	}
}

func (w *Worker) newDeleter(logger log.Logger, tombstone *metastorev1.Tombstones) *deleter {
	return &deleter{
		logger:    logger,
		bucket:    w.storage,
		metrics:   w.metrics,
		tombstone: tombstone,
	}
}

type deleter struct {
	logger    log.Logger
	bucket    objstore.Bucket
	metrics   *workerMetrics
	tombstone *metastorev1.Tombstones
	wg        sync.WaitGroup
}

func (d *deleter) run(ctx context.Context, p *deleterPool) {
	if t := d.tombstone.GetBlocks(); t != nil {
		d.handleBlockTombstones(ctx, p, t)
	}
	if t := d.tombstone.GetShard(); t != nil {
		d.handleShardTombstone(ctx, p, t)
	}
}

func (d *deleter) wait() { d.wg.Wait() }

func (d *deleter) handleBlockTombstones(ctx context.Context, pool *deleterPool, t *metastorev1.BlockTombstones) {
	logger := log.With(d.logger, "tombstone_name", t.Name)
	level.Info(logger).Log("msg", "deleting blocks", "blocks", strings.Join(t.Blocks, " "))
	for _, b := range t.Blocks {
		d.wg.Add(1)
		pool.run(func() {
			defer d.wg.Done()
			d.delete(ctx, block.BuildObjectPath(t.Tenant, t.Shard, t.CompactionLevel, b))
		})
	}
}

func (d *deleter) handleShardTombstone(ctx context.Context, pool *deleterPool, t *metastorev1.ShardTombstone) {
	// It's safe to delete blocks in the shard that are older than the
	// maximum time specified in the tombstone.
	minTime := time.Unix(0, t.Timestamp)
	maxTime := minTime.Add(time.Duration(t.Duration))
	dir := block.BuildObjectDir(t.Tenant, t.Shard)

	logger := log.With(d.logger, "tombstone_name", t.Name)
	level.Info(logger).Log("msg", "cleaning up shard", "max_time", maxTime, "dir", dir)

	// Workaround for MinIO/S3 ListObjects: if we stop consuming before cancelling,
	// the producer goroutine can block on a final send. Cancel first and keep
	// draining so the producer exits cleanly. Thanos Iter does not drain on early
	// return, so we do it here.
	// See: https://github.com/minio/minio-go/blame/f64cdbde257f48f1a44b0f5aeee0475bad7e0e8d/api-list.go#L784
	iterCtx, iterCancel := context.WithCancel(ctx)
	defer iterCancel()

	deleteBlock := func(path string) error {
		// After we cancel iterCtx, the provider (e.g., MinIO ListObjects) may do
		// one final blocking send on its results channel. Returning nil here keeps
		// draining without scheduling new work so the producer isn't left blocked.
		if iterCtx.Err() != nil {
			return nil
		}
		blockID, err := block.ParseBlockIDFromPath(path)
		if err != nil {
			level.Warn(logger).Log("msg", "failed to parse block ID from path", "path", path, "err", err)
			return nil
		}
		// Note that although we could skip blocks that are older than the
		// minimum time, we do not do it here: we want to make sure we deleted
		// everything before the maximum time, as previous jobs could fail
		// to do so. In the worst case, this may result in a competition between
		// workers that try to clean up the same shard. This is not an issue
		// in practice, because there are not so many cleanup jobs for the
		// same shard are running concurrently, and the cleanup is fast.
		blockTs := time.UnixMilli(int64(blockID.Time()))
		if !blockTs.Before(maxTime) {
			level.Debug(logger).Log("msg", "reached range end, exiting", "path", path)
			// Cancel the iterator so the underlying producer exits promptly.
			// Keep consuming to drain any buffered items and allow the producer's
			// final send on ctx.Done() to be received.
			iterCancel()
			return nil
		}
		d.wg.Add(1)
		pool.run(func() {
			defer d.wg.Done()
			d.delete(ctx, path)
		})
		return nil
	}

	if err := d.bucket.Iter(iterCtx, dir, deleteBlock, thanosstore.WithRecursiveIter()); err != nil {
		if errors.Is(err, context.Canceled) {
			// Expected when the iteration context is cancelled.
			return
		}
		// It's only possible if the error is returned by the iterator itself.
		level.Error(logger).Log("msg", "failed to cleanup shard", "err", err)
	}
}

func (d *deleter) delete(ctx context.Context, path string) {
	var statusName status
	switch err := d.bucket.Delete(ctx, path); {
	case err == nil:
		statusName = statusSuccess

	case objstore.IsNotExist(d.bucket, err):
		level.Info(d.logger).Log("msg", "block not found while attempting to delete it", "path", path, "err", err)
		statusName = statusBlockNotFound

	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		level.Warn(d.logger).Log("msg", "block delete attempt canceled", "path", path, "err", err)
		statusName = statusCanceled

	default:
		level.Error(d.logger).Log("msg", "failed to delete block", "path", path, "err", err)
		statusName = statusFailure
	}

	d.metrics.blocksDeleted.WithLabelValues(string(statusName)).Inc()
}

type deleterPool struct {
	deletersWg sync.WaitGroup
	stop       chan struct{}

	threadsWg sync.WaitGroup
	queue     chan func()
}

func newDeleterPool(threads int) *deleterPool {
	p := &deleterPool{
		queue: make(chan func(), threads),
		stop:  make(chan struct{}),
	}
	p.threadsWg.Add(threads)
	for i := 0; i < threads; i++ {
		go func() {
			defer p.threadsWg.Done()
			for fn := range p.queue {
				fn()
			}
		}()
	}
	return p
}

// If too many tombstones are created for the same tenant-shard, of if there
// are too many blocks to delete so a single worker does not cope up, multiple
// workers may end up deleting same blocks as they process the shard from the
// very beginning. The timeout aims to reduce the competition factor: at any
// time, the number of workers that cleanup the same shard is limited. This is
// difficult to achieve in practice, and may happen if the retention is enabled
// for the first time, and large number of blocks are deleted at once.
func (p *deleterPool) deleterContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx := context.Background()
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func (p *deleterPool) add(deleter *deleter, timeout time.Duration) {
	ctx, cancel := p.deleterContext(timeout)
	done := make(chan struct{})
	p.deletersWg.Add(1)
	go func() {
		deleter.run(ctx, p)
		deleter.wait()
		p.deletersWg.Done()
		// Notify the other goroutine that the deleter is done
		// and there's no need to wait for it anymore.
		close(done)
	}()
	go func() {
		// Wait for the deleter to finish or for the stop signal,
		// or for the timeout to expire, whichever comes first.
		defer cancel()
		select {
		case <-done:
		case <-ctx.Done():
		case <-p.stop:
			// We don't want to halt the deletion abruptly when
			// the worker is stopped. In most cases, the deletion
			// will be finished by the time the worker is stopped.
			// Otherwise, we may wait up to CleanupMaxDuration.
			select {
			case <-done:
			case <-ctx.Done():
			}
		}
	}()
}

func (p *deleterPool) run(fn func()) { p.queue <- fn }

// It is guaranteed that no [add] calls will be made at this point:
// all compaction jobs are done, and no new jobs can be queued.
func (p *deleterPool) close() {
	// Wait for all the deleters to finish.
	close(p.stop)
	p.deletersWg.Wait()
	// No new deletions can be queued.
	// We can close the queue now.
	close(p.queue)
	p.threadsWg.Wait()
}
