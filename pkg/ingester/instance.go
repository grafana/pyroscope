package ingester

import (
	"context"
	"path"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	phlareobj "github.com/grafana/phlare/pkg/objstore"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	"github.com/grafana/phlare/pkg/phlaredb/shipper"
)

type instance struct {
	*phlaredb.PhlareDB
	shipper     *shipper.Shipper
	shipperLock sync.Mutex
	logger      log.Logger
	reg         prometheus.Registerer

	cancel   context.CancelFunc
	wg       sync.WaitGroup
	tenantID string
}

func newInstance(phlarectx context.Context, cfg phlaredb.Config, tenantID string, storageBucket phlareobj.Bucket, limiter Limiter) (*instance, error) {
	cfg.DataPath = path.Join(cfg.DataPath, tenantID)

	phlarectx = phlarecontext.WrapTenant(phlarectx, tenantID)
	db, err := phlaredb.New(phlarectx, cfg, limiter)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(phlarectx)
	inst := &instance{
		PhlareDB: db,
		logger:   phlarecontext.Logger(phlarectx),
		reg:      phlarecontext.Registry(phlarectx),
		cancel:   cancel,
		tenantID: tenantID,
	}
	// Todo we should not ship when using filesystem storage.
	if storageBucket != nil {
		inst.shipper = shipper.New(
			inst.logger,
			inst.reg,
			db,
			phlareobj.NewPrefixedBucket(storageBucket, tenantID+"/phlaredb"),
			block.IngesterSource,
			false,
			false,
		)
	}
	go inst.loop(ctx)
	return inst, nil
}

func (i *instance) loop(ctx context.Context) {
	i.wg.Add(1)
	defer func() {
		i.runShipper(context.Background()) // Run shipper one last time.
		i.wg.Done()
	}()
	// run shipper periodically and at start-up
	shipperTicker := time.NewTicker(5 * time.Minute)
	defer shipperTicker.Stop()
	go func() {
		i.runShipper(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-shipperTicker.C: // run shipper loop
			i.runShipper(ctx)
		}
	}
}

func (i *instance) runShipper(ctx context.Context) {
	i.shipperLock.Lock()
	defer i.shipperLock.Unlock()
	if i.shipper == nil {
		return
	}
	uploaded, err := i.shipper.Sync(ctx)
	if err != nil {
		level.Error(i.logger).Log("msg", "shipper run failed", "err", err)
	} else {
		level.Info(i.logger).Log("msg", "shipper finished", "uploaded_blocks", uploaded)
	}
}

func (i *instance) Stop() error {
	err := i.PhlareDB.Close()
	i.cancel()
	i.wg.Wait()
	return err
}
