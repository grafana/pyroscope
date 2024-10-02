package dlq

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	segmentstorage "github.com/grafana/pyroscope/pkg/experiment/ingester/storage"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftleader"
	"github.com/thanos-io/objstore"
)

type RecoveryConfig struct {
	Period time.Duration
}

type LocalServer interface {
	AddRecoveredBlock(context.Context, *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)
}

type Recovery struct {
	cfg    RecoveryConfig
	l      log.Logger
	srv    LocalServer
	bucket objstore.Bucket

	started bool
	wg      sync.WaitGroup
	m       sync.Mutex
	cancel  func()
}

func NewRecovery(cfg RecoveryConfig, l log.Logger, srv LocalServer, bucket objstore.Bucket) *Recovery {
	return &Recovery{
		cfg:    cfg,
		l:      l,
		srv:    srv,
		bucket: bucket,
	}
}

func (r *Recovery) Start() {
	r.m.Lock()
	defer r.m.Unlock()
	if r.started {
		r.l.Log("msg", "recovery already started")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.started = true
	go r.recoverLoop(ctx)
	r.l.Log("msg", "recovery started")
}

func (r *Recovery) Stop() {
	r.m.Lock()
	defer r.m.Unlock()
	if !r.started {
		r.l.Log("msg", "recovery already stopped")
		return
	}
	r.cancel()
	r.wg.Wait()
	r.started = false
	r.l.Log("msg", "recovery stopped")
}

func (r *Recovery) recoverLoop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.Period)
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
		level.Error(r.l).Log("msg", "failed to iterate over dlq", "err", err)
	}
}

func (r *Recovery) recover(ctx context.Context, metaPath string) error {
	fields := strings.Split(metaPath, "/")
	if len(fields) != 5 {
		r.l.Log("msg", "unexpected path", "path", metaPath)
		return nil
	}
	sshard := fields[1]
	ulid := fields[3]
	meta, err := r.get(ctx, metaPath)
	if err != nil {
		level.Error(r.l).Log("msg", "failed to get block meta", "err", err, "path", metaPath)
		return nil
	}
	shard, _ := strconv.ParseUint(sshard, 10, 64)
	if ulid != meta.Id || meta.Shard != uint32(shard) {
		level.Error(r.l).Log("msg", "unexpected block meta", "path", metaPath, "meta", fmt.Sprintf("%+v", meta))
		return nil
	}
	_, err = r.srv.AddRecoveredBlock(ctx, &metastorev1.AddBlockRequest{
		Block: meta,
	})
	if err != nil {
		if raftleader.IsRaftLeadershipError(err) {
			return err
		}
		level.Error(r.l).Log("msg", "failed to add block", "err", err, "path", metaPath)
		return nil
	}
	err = r.bucket.Delete(ctx, metaPath)
	if err != nil {
		level.Error(r.l).Log("msg", "failed to delete block meta", "err", err, "path", metaPath)
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
