package ingester

import (
	"context"
	"path"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/fire/pkg/firedb"
	"github.com/grafana/fire/pkg/firedb/block"
	"github.com/grafana/fire/pkg/firedb/shipper"
	fireobjstore "github.com/grafana/fire/pkg/objstore"
)

type instance struct {
	*firedb.FireDB
	shipper     *shipper.Shipper
	shipperLock sync.Mutex
	logger      log.Logger
	reg         prometheus.Registerer

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newInstance(cfg firedb.Config, instanceID string, logger log.Logger, storageBucket fireobjstore.Bucket, reg prometheus.Registerer) (*instance, error) {
	cfg.DataPath = path.Join(cfg.DataPath, instanceID)
	reg = prometheus.WrapRegistererWith(prometheus.Labels{"tenant": instanceID}, reg)
	db, err := firedb.New(cfg, logger, reg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	inst := &instance{
		FireDB: db,
		logger: log.With(logger, "tenant", instanceID),
		reg:    reg,
		cancel: cancel,
	}
	if storageBucket != nil {
		inst.shipper = shipper.New(
			logger,
			reg,
			db,
			fireobjstore.BucketWithPrefix(storageBucket, instanceID+"/firedb"),
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
		level.Info(i.logger).Log("msg", "shipper finshed", "uploaded_blocks", uploaded)
	}
}

func (i *instance) Stop() error {
	err := i.FireDB.Close()
	i.cancel()
	i.wg.Wait()
	return err
}
