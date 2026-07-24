package dlq

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode"
)

type Config struct {
	CheckInterval time.Duration `yaml:"dlq_recovery_check_interval" category:"advanced"`
}

func (c *Config) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.CheckInterval, prefix+"dlq-recovery-check-interval", 15*time.Second, "Dead Letter Queue check interval. 0 to disable.")
}

type Metastore interface {
	AddRecoveredBlock(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)
}

type Recovery struct {
	config    Config
	logger    log.Logger
	metastore Metastore
	bucket    objstore.Bucket
	metrics   *metrics

	started bool
	cancel  func()
	m       sync.Mutex
}

func NewRecovery(logger log.Logger, config Config, metastore Metastore, bucket objstore.Bucket, reg prometheus.Registerer) *Recovery {
	return &Recovery{
		config:    config,
		logger:    logger,
		metastore: metastore,
		bucket:    bucket,
		metrics:   newMetrics(reg),
	}
}

func (r *Recovery) Start() {
	if r.config.CheckInterval == 0 {
		return
	}
	r.m.Lock()
	defer r.m.Unlock()
	if r.started {
		r.logger.Log("msg", "recovery already started")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.started = true
	go r.recoverLoop(ctx)
	r.logger.Log("msg", "recovery started")
}

func (r *Recovery) Stop() {
	if r.config.CheckInterval == 0 {
		return
	}
	r.m.Lock()
	defer r.m.Unlock()
	if !r.started {
		r.logger.Log("msg", "recovery already stopped")
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.started = false
	r.logger.Log("msg", "recovery stopped")
}

func (r *Recovery) recoverLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.recoverTick(ctx)
		}
	}
}

func (r *Recovery) recoverTick(ctx context.Context) {
	err := r.bucket.Iter(ctx, block.DirNameDLQ, func(path string) error {
		return r.recover(ctx, path)
	}, objstore.WithRecursiveIter())
	if err != nil {
		level.Error(r.logger).Log("msg", "failed to recover block metadata", "err", err)
	}
}

// recover processes a single DLQ object.
//
// It returns a non-nil error only when the whole recovery sweep must be
// stopped (e.g. context cancellation or a raft leadership change); in that
// case the current object is left in place to be retried later. All per-object
// failures (stray keys, empty/corrupt objects, transient read or metastore
// errors) are handled here and reported via metrics, and nil is returned so
// that iteration continues over the remaining DLQ entries. Otherwise a single
// bad object would block the recovery of every entry that follows it.
func (r *Recovery) recover(ctx context.Context, path string) error {
	// Pyroscope only ever writes DLQ objects at
	// "dlq/<shard>/<tenant>/<block-ulid>/meta.pb" (see block.MetadataDLQObjectPath).
	// Anything else under the prefix was not written by us: most commonly a
	// zero-byte "folder placeholder" such as "dlq/" or "dlq/1/" created by S3
	// tooling, multipart leftovers, or operator/backup artifacts. Such stray
	// keys can never be read as block metadata and, before this guard, caused
	// an EOF read error that aborted the whole sweep. We skip them and keep
	// iterating; we do not delete them.
	if !isDLQMetadataPath(path) {
		r.metrics.recoveryAttempts.WithLabelValues("stray").Inc()
		level.Warn(r.logger).Log("msg", "unexpected object in DLQ; skipping", "path", path)
		return nil
	}

	b, err := r.readObject(ctx, path)
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		r.metrics.recoveryAttempts.WithLabelValues("canceled").Inc()
		return err
	case r.bucket.IsObjNotFoundErr(err):
		// This is somewhat opportunistic: the error is likely caused by a competing recovery
		// process that has already recovered the block, before we've discovered that the
		// leadership has changed.
		r.metrics.recoveryAttempts.WithLabelValues("not_found").Inc()
		level.Warn(r.logger).Log("msg", "block metadata not found; skipping", "path", path)
		return nil
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		// A zero-byte meta.pb object: object stores (e.g. S3) surface an empty
		// object as EOF on the first read. It can never unmarshal into a valid
		// block, so retrying forever only spams logs. Delete it and continue.
		r.metrics.recoveryAttempts.WithLabelValues("empty").Inc()
		level.Warn(r.logger).Log("msg", "empty block metadata; deleting", "path", path)
		if delErr := r.bucket.Delete(ctx, path); delErr != nil {
			level.Warn(r.logger).Log("msg", "failed to delete empty block metadata", "err", delErr, "path", path)
		}
		return nil
	default:
		// This is somewhat opportunistic, as we don't know if the error is transient or not.
		// we should consider an explicit retry mechanism with backoff and a limit on the
		// number of attempts. We keep the object and continue the sweep so that a single
		// transient failure does not block the recovery of the remaining entries.
		r.metrics.recoveryAttempts.WithLabelValues("read_error").Inc()
		level.Warn(r.logger).Log("msg", "failed to read block metadata; to be retried", "err", err, "path", path)
		return nil
	}

	if len(b) == 0 {
		// Same as the EOF case above, but for object stores that return an empty
		// reader without an error for zero-byte objects.
		r.metrics.recoveryAttempts.WithLabelValues("empty").Inc()
		level.Warn(r.logger).Log("msg", "empty block metadata; deleting", "path", path)
		if delErr := r.bucket.Delete(ctx, path); delErr != nil {
			level.Warn(r.logger).Log("msg", "failed to delete empty block metadata", "err", delErr, "path", path)
		}
		return nil
	}

	var meta metastorev1.BlockMeta
	if err := meta.UnmarshalVT(b); err != nil {
		// Corrupt metadata can never be recovered; deleting it prevents endless
		// retries and log spam. Matches the pre-refactor behaviour.
		r.metrics.recoveryAttempts.WithLabelValues("unmarshal_error").Inc()
		level.Error(r.logger).Log("msg", "failed to unmarshal block metadata; deleting", "err", err, "path", path)
		if delErr := r.bucket.Delete(ctx, path); delErr != nil {
			level.Warn(r.logger).Log("msg", "failed to delete corrupt block metadata", "err", delErr, "path", path)
		}
		return nil
	}

	switch _, err := r.metastore.AddRecoveredBlock(ctx, &metastorev1.AddBlockRequest{Block: &meta}); {
	case err == nil:
		r.metrics.recoveryAttempts.WithLabelValues("success").Inc()
		level.Debug(r.logger).Log("msg", "successfully recovered block from DLQ", "block_id", meta.Id, "path", path)
		// The block is now recorded in the metastore; remove it from the DLQ.
		if delErr := r.bucket.Delete(ctx, path); delErr != nil {
			level.Warn(r.logger).Log("msg", "failed to delete block metadata", "err", delErr, "path", path)
		}
		return nil
	case status.Code(err) == codes.InvalidArgument:
		// The metastore permanently rejected the metadata; deleting it prevents
		// endless retries. Matches the pre-refactor behaviour.
		r.metrics.recoveryAttempts.WithLabelValues("invalid_metadata").Inc()
		level.Error(r.logger).Log("msg", "block metadata rejected by metastore; deleting", "err", err, "block_id", meta.Id, "path", path)
		if delErr := r.bucket.Delete(ctx, path); delErr != nil {
			level.Warn(r.logger).Log("msg", "failed to delete rejected block metadata", "err", delErr, "path", path)
		}
		return nil
	case raftnode.IsRaftLeadershipError(err):
		r.metrics.recoveryAttempts.WithLabelValues("leadership_change").Inc()
		level.Warn(r.logger).Log("msg", "leadership change; recovery interrupted", "err", err, "path", path)
		return err
	default:
		r.metrics.recoveryAttempts.WithLabelValues("metastore_error").Inc()
		level.Error(r.logger).Log("msg", "failed to add block metadata; to be retried", "err", err, "path", path)
		return nil
	}
}

func (r *Recovery) readObject(ctx context.Context, path string) ([]byte, error) {
	rc, err := r.bucket.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rc.Close()
	}()
	return io.ReadAll(rc)
}

// isDLQMetadataPath reports whether path matches the exact layout that
// block.MetadataDLQObjectPath produces:
//
//	dlq/<shard>/<tenant>/<block-ulid>/meta.pb
//
// Anything else found under the "dlq/" prefix (folder placeholders, multipart
// leftovers, operator artifacts, etc.) is not a DLQ entry written by Pyroscope
// and must be ignored rather than read as block metadata.
func isDLQMetadataPath(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) != 5 {
		return false
	}
	if parts[0] != block.DirNameDLQ {
		return false
	}
	// parts[1] shard, parts[2] tenant: non-empty, structural.
	if parts[1] == "" || parts[2] == "" {
		return false
	}
	if parts[4] != block.FileNameMetadataObject {
		return false
	}
	// parts[3] must be a valid block ULID.
	if _, err := ulid.Parse(parts[3]); err != nil {
		return false
	}
	return true
}
