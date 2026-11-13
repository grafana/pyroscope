package recording

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/v2/pkg/util"
)

const (
	IndexPath        = "recording_rules_index.v1.json"
	indexConcurrency = 16
)

// Index maps every tenant that has stored recording rules to the generation of
// its rules file.
type Index struct {
	GeneratedAt int64            `json:"generated_at"`
	Tenants     map[string]int64 `json:"tenants"`
}

var ErrIndexNotFound = errors.New("recording rules index not found")

type IndexStore struct{ bucket objstore.Bucket }

func NewIndexStore(bucket objstore.Bucket) *IndexStore { return &IndexStore{bucket: bucket} }

func (s *IndexStore) Load(ctx context.Context) (*Index, error) {
	r, err := s.bucket.Get(ctx, IndexPath)
	if err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, ErrIndexNotFound
		}
		return nil, err
	}
	defer func() { _ = r.Close() }()

	var idx Index
	if err := json.NewDecoder(r).Decode(&idx); err != nil {
		return nil, fmt.Errorf("decoding recording rules index: %w", err)
	}
	if idx.Tenants == nil {
		idx.Tenants = map[string]int64{}
	}
	return &idx, nil
}

func (s *IndexStore) Store(ctx context.Context, idx *Index) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return s.bucket.Upload(ctx, IndexPath, bytes.NewReader(data))
}

// BuildIndex lists the tenants in the bucket and reads the rules file of each.
// Any read failure fails the build, so a partial index is never produced.
func BuildIndex(ctx context.Context, bucket objstore.Bucket, logger log.Logger) (*Index, error) {
	var tenants []string
	err := bucket.Iter(ctx, "", func(name string) error {
		if tenant, ok := strings.CutSuffix(name, "/"); ok {
			tenants = append(tenants, tenant)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}

	var (
		mu  sync.Mutex
		idx = &Index{
			GeneratedAt: time.Now().UnixNano(),
			Tenants:     make(map[string]int64),
		}
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(indexConcurrency)
	for _, tenant := range tenants {
		g.Go(func() error {
			stored, err := ReadStoredRules(gctx, bucket, logger, tenant)
			if err != nil {
				return fmt.Errorf("reading rules of tenant %q: %w", tenant, err)
			}
			if len(stored.Rules) == 0 {
				return nil
			}
			mu.Lock()
			idx.Tenants[tenant] = stored.Generation
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return idx, nil
}

type IndexBuilder struct {
	bucket   objstore.Bucket
	store    *IndexStore
	logger   log.Logger
	interval time.Duration
}

func NewIndexBuilder(bucket objstore.Bucket, logger log.Logger, interval time.Duration) *IndexBuilder {
	return &IndexBuilder{bucket: bucket, store: NewIndexStore(bucket), logger: logger, interval: interval}
}

func (b *IndexBuilder) Rebuild(ctx context.Context) error {
	idx, err := BuildIndex(ctx, b.bucket, b.logger)
	if err != nil {
		return err
	}
	return b.store.Store(ctx, idx)
}

func (b *IndexBuilder) Run(ctx context.Context) {
	t := time.NewTicker(b.interval)
	defer t.Stop()
	for {
		util.Recover(func() {
			if err := b.Rebuild(ctx); err != nil && ctx.Err() == nil {
				_ = level.Error(b.logger).Log("msg", "failed to rebuild recording rules index", "err", err)
			}
		})
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
