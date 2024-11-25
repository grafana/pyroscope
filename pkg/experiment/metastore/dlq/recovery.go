package dlq

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	segmentstorage "github.com/grafana/pyroscope/pkg/experiment/ingester/storage"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
)

type RecoveryConfig struct {
	Period time.Duration `yaml:"dlq_recovery_check_interval"`
}

func (c *RecoveryConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&c.Period, prefix+"dlq-recovery-check-interval", 15*time.Second, "Dead Letter Queue check interval.")
}

type LocalServer interface {
	AddRecoveredBlock(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)
}

type Recovery struct {
	config    RecoveryConfig
	logger    log.Logger
	metastore LocalServer
	bucket    objstore.Bucket

	m       sync.Mutex
	started bool
	cancel  func()
}

func NewRecovery(logger log.Logger, config RecoveryConfig, metastore LocalServer, bucket objstore.Bucket) *Recovery {
	return &Recovery{
		config:    config,
		logger:    logger,
		metastore: metastore,
		bucket:    bucket,
	}
}

func (r *Recovery) Start() {
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
	r.m.Lock()
	defer r.m.Unlock()
	if !r.started {
		r.logger.Log("msg", "recovery already stopped")
		return
	}
	r.cancel()
	r.started = false
	r.logger.Log("msg", "recovery stopped")
}

func (r *Recovery) recoverLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.Period)
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
	err := r.bucket.Iter(ctx, segmentstorage.PathDLQ, func(metaPath string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return r.recover(ctx, metaPath)
	}, objstore.WithRecursiveIter)
	if err != nil {
		level.Error(r.logger).Log("msg", "failed to iterate over dlq", "err", err)
	}
}

func (r *Recovery) recover(ctx context.Context, metaPath string) error {
	fields := strings.Split(metaPath, "/")
	if len(fields) != 5 {
		r.logger.Log("msg", "unexpected path", "path", metaPath)
		return nil
	}
	sshard := fields[1]
	ulid := fields[3]
	meta, err := r.get(ctx, metaPath)
	if err != nil {
		level.Error(r.logger).Log("msg", "failed to get block meta", "err", err, "path", metaPath)
		return nil
	}
	shard, _ := strconv.ParseUint(sshard, 10, 64)
	if ulid != meta.Id || meta.Shard != uint32(shard) {
		level.Error(r.logger).Log("msg", "unexpected block meta", "path", metaPath, "meta", fmt.Sprintf("%+v", meta))
		return nil
	}
	if _, err = r.metastore.AddRecoveredBlock(ctx, &metastorev1.AddBlockRequest{Block: meta}); err != nil {
		if raftnode.IsRaftLeadershipError(err) {
			return err
		}
		level.Error(r.logger).Log("msg", "failed to add block", "err", err, "path", metaPath)
		return nil
	}
	err = r.bucket.Delete(ctx, metaPath)
	if err != nil {
		level.Error(r.logger).Log("msg", "failed to delete block meta", "err", err, "path", metaPath)
	}
	return nil
}

func (r *Recovery) get(ctx context.Context, metaPath string) (*metastorev1.BlockMeta, error) {
	meta, err := r.bucket.Get(ctx, metaPath)
	if err != nil {
		return nil, err
	}
	metaBytes, err := io.ReadAll(meta)
	if err != nil {
		return nil, err
	}
	recovered := new(metastorev1.BlockMeta)
	err = recovered.UnmarshalVT(metaBytes)
	if err != nil {
		return nil, err
	}
	return recovered, nil
}
