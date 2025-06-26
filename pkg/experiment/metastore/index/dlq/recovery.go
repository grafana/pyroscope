package dlq

import (
	"context"
	"errors"
	"flag"
	"io"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
)

type Config struct {
	CheckInterval time.Duration `yaml:"dlq_recovery_check_interval"`
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

	started bool
	cancel  func()
	m       sync.Mutex
}

func NewRecovery(logger log.Logger, config Config, metastore Metastore, bucket objstore.Bucket) *Recovery {
	return &Recovery{
		config:    config,
		logger:    logger,
		metastore: metastore,
		bucket:    bucket,
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

func (r *Recovery) recover(ctx context.Context, path string) (err error) {
	defer func() {
		if err == nil {
			// In case we return no error, the block is considered recovered and will be deleted.
			if delErr := r.bucket.Delete(ctx, path); delErr != nil {
				level.Warn(r.logger).Log("msg", "failed to delete block metadata", "err", delErr, "path", path)
			}
		}
	}()

	b, err := r.readObject(ctx, path)
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		return err
	case r.bucket.IsObjNotFoundErr(err):
		// This is somewhat opportunistic: the error is likely caused by a competing recovery
		// process that has already recovered the block, before we've discovered that the
		// leadership has changed.
		level.Warn(r.logger).Log("msg", "block metadata not found; skipping", "path", path)
		return nil
	default:
		// This is somewhat opportunistic, as we don't know if the error is transient or not.
		// we should consider an explicit retry mechanism with backoff and a limit on the
		// number of attempts.
		level.Warn(r.logger).Log("msg", "failed to read block metadata; to be retried", "err", err, "path", path)
		return err
	}

	var meta metastorev1.BlockMeta
	if err = meta.UnmarshalVT(b); err != nil {
		level.Error(r.logger).Log("msg", "invalid block metadata; skipping", "err", err, "path", path)
		return nil
	}

	switch _, err = r.metastore.AddRecoveredBlock(ctx, &metastorev1.AddBlockRequest{Block: &meta}); {
	case err == nil:
		return nil
	case status.Code(err) == codes.InvalidArgument:
		level.Error(r.logger).Log("msg", "invalid block metadata", "err", err, "path", path)
		return nil
	case raftnode.IsRaftLeadershipError(err):
		level.Warn(r.logger).Log("msg", "leadership change; recovery interrupted", "err", err, "path", path)
		return err
	default:
		level.Error(r.logger).Log("msg", "failed to add block metadata; to be retried", "err", err, "path", path)
		return err
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
