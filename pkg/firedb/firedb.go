package firedb

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	"github.com/grafana/fire/pkg/firedb/block"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/objstore/client"
	"github.com/grafana/fire/pkg/objstore/providers/filesystem"
)

type Config struct {
	DataPath      string
	BlockDuration time.Duration
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.DataPath, "firedb.data-path", "./data", "Directory used for local storage.")
	f.DurationVar(&cfg.BlockDuration, "firedb.block-duration", 30*time.Minute, "Block duration.")
}

type FireDB struct {
	services.Service

	cfg    *Config
	reg    prometheus.Registerer
	logger log.Logger
	stopCh chan struct{}

	headLock      sync.RWMutex
	head          *Head
	headMetrics   *headMetrics
	headFlushTime time.Time

	blockQuerier *BlockQuerier
}

func New(cfg *Config, logger log.Logger, reg prometheus.Registerer) (*FireDB, error) {
	headMetrics := newHeadMetrics(reg)
	f := &FireDB{
		cfg:         cfg,
		reg:         reg,
		logger:      logger,
		stopCh:      make(chan struct{}, 0),
		headMetrics: headMetrics,
	}
	if _, err := f.initHead(); err != nil {
		return nil, err
	}
	f.Service = services.NewBasicService(f.starting, f.running, f.stopping)

	fs, err := filesystem.NewBucket(cfg.DataPath)
	if err != nil {
		return nil, err
	}
	bucketReader, err := client.ReaderAtBucket(pathLocal, fs, prometheus.WrapRegistererWithPrefix("firedb", reg))
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(f.LocalDataPath(), 0o777); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", f.LocalDataPath(), err)
	}

	f.blockQuerier = NewBlockQuerier(logger, bucketReader)

	// do an initial querier sync
	ctx := context.Background()
	if err := f.blockQuerier.Sync(ctx); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *FireDB) LocalDataPath() string {
	return filepath.Join(f.cfg.DataPath, pathLocal)
}

func (f *FireDB) BlockMetas(ctx context.Context) ([]*block.Meta, error) {
	return f.blockQuerier.BlockMetas(ctx)
}

func (f *FireDB) runBlockQuerierSync(ctx context.Context) {
	if err := f.blockQuerier.Sync(ctx); err != nil {
		level.Error(f.logger).Log("msg", "sync blocks failed", "err", err)
	}
}

func (f *FireDB) loop() {
	var (
		blockScanTicker = time.NewTicker(5 * time.Minute)
		blockScanManual = make(chan struct{}, 1)
	)
	defer func() {
		close(blockScanManual)
		blockScanTicker.Stop()
	}()

	for {
		ctx := context.Background()

		f.headLock.RLock()
		timeToFlush := f.headFlushTime.Sub(time.Now())
		f.headLock.RUnlock()

		select {
		case <-f.stopCh:
			return
		case <-time.After(timeToFlush):
			if err := f.Flush(ctx); err != nil {
				level.Error(f.logger).Log("msg", "flushing head block failed", "err", err)
				continue
			}
			f.runBlockQuerierSync(ctx)
		case <-blockScanTicker.C:
			f.runBlockQuerierSync(ctx)
		}
	}
}

func (f *FireDB) starting(ctx context.Context) error {
	go f.loop()
	return nil
}

func (f *FireDB) running(ctx context.Context) error {
	select {
	// wait until service is asked to stop
	case <-ctx.Done():
		// stop
		close(f.stopCh)
	}
	return nil
}

func (f *FireDB) stopping(_ error) error {
	errs := multierror.New()
	if err := f.blockQuerier.Close(); err != nil {
		errs.Add(err)
	}
	if err := f.Close(context.Background()); err != nil {
		errs.Add(err)
	}
	return errs.Err()
}

func (f *FireDB) Head() *Head {
	f.headLock.RLock()
	defer f.headLock.RUnlock()
	return f.head
}

type profileSelecter interface {
	SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error)
	InRange(start, end model.Time) bool
}

type profileSelecters []profileSelecter

func (ps profileSelecters) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	// first check which profileSelecters are in range before executing
	ps = lo.Filter(ps, func(e profileSelecter, _ int) bool {
		return e.InRange(
			model.Time(req.Msg.Start),
			model.Time(req.Msg.End),
		)
	})

	results := make([]*ingestv1.SelectProfilesResponse, len(ps))

	g, ctx := errgroup.WithContext(ctx)
	// todo not sure this help on disk IO
	g.SetLimit(16)

	query := func(ctx context.Context, pos int) {
		g.Go(func() error {
			resp, err := ps[pos].SelectProfiles(ctx, req)
			if err != nil {
				return err
			}

			results[pos] = resp.Msg
			return nil
		})
	}

	for pos := range ps {
		query(ctx, pos)
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return connect.NewResponse(mergeSelectProfilesResponse(results...)), nil
}

func mergeSelectProfilesResponse(responses ...*ingestv1.SelectProfilesResponse) *ingestv1.SelectProfilesResponse {
	var (
		result    *ingestv1.SelectProfilesResponse
		posByName map[string]int32
	)

	for _, resp := range responses {
		// skip empty results
		if resp == nil || len(resp.Profiles) == 0 {
			continue
		}

		// first non-empty result result
		if result == nil {
			result = resp
			continue
		}

		// build up the lookup map the first time
		if posByName == nil {
			posByName = make(map[string]int32)
			for idx, n := range result.FunctionNames {
				posByName[n] = int32(idx)
			}
		}

		// lookup and add missing functionNames
		var (
			rewrite = make([]int32, len(resp.FunctionNames))
			ok      bool
		)
		for idx, n := range resp.FunctionNames {
			rewrite[idx], ok = posByName[n]
			if ok {
				continue
			}

			// need to add functionName to list
			rewrite[idx] = int32(len(result.FunctionNames))
			result.FunctionNames = append(result.FunctionNames, n)
		}

		// rewrite existing function ids, by building a list of unique slices
		functionIDsUniq := make(map[*int32][]int32)
		for _, profile := range resp.Profiles {
			for _, sample := range profile.Stacktraces {
				if len(sample.FunctionIds) == 0 {
					continue
				}
				functionIDsUniq[&sample.FunctionIds[0]] = sample.FunctionIds
			}
		}
		// now rewrite those ids in slices
		for _, slice := range functionIDsUniq {
			for idx, functionID := range slice {
				slice[idx] = rewrite[functionID]
			}
		}
		result.Profiles = append(result.Profiles, resp.Profiles...)
	}

	// ensure nil will always be the empty response
	if result == nil {
		result = &ingestv1.SelectProfilesResponse{}
	}

	return result
}

func (f *FireDB) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	sources := append(f.blockQuerier.profileSelecters(), f.Head())
	return sources.SelectProfiles(ctx, req)
}

func (f *FireDB) initHead() (oldHead *Head, err error) {
	f.headLock.Lock()
	defer f.headLock.Unlock()
	oldHead = f.head
	f.headFlushTime = time.Now().UTC().Truncate(f.cfg.BlockDuration).Add(f.cfg.BlockDuration)
	f.head, err = NewHead(f.cfg.DataPath, headWithMetrics(f.headMetrics), HeadWithLogger(f.logger))
	if err != nil {
		return oldHead, err
	}
	return oldHead, nil
}

func (f *FireDB) Flush(ctx context.Context) error {
	oldHead, err := f.initHead()
	if err != nil {
		return err
	}

	if oldHead == nil {
		return nil
	}
	return oldHead.Flush(ctx)
}

func (f *FireDB) Close(ctx context.Context) error {
	return f.head.Flush(ctx)
}
