package ingester

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/klauspost/compress/gzip"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/fire/pkg/firedb"
	"github.com/grafana/fire/pkg/firedb/block"
	"github.com/grafana/fire/pkg/firedb/shipper"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	fireobjstore "github.com/grafana/fire/pkg/objstore"
	"github.com/grafana/fire/pkg/tenant"
	"github.com/grafana/fire/pkg/util"
)

type Config struct {
	LifecyclerConfig ring.LifecyclerConfig `yaml:"lifecycler,omitempty"`
}

// RegisterFlags registers the flags.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.LifecyclerConfig.RegisterFlags(f, util.Logger)
}

func (cfg *Config) Validate() error {
	return nil
}

type Ingester struct {
	services.Service

	cfg    Config
	logger log.Logger

	lifecycler        *ring.Lifecycler
	lifecyclerWatcher *services.FailureWatcher
	fireDB            *firedb.FireDB

	shipper     *shipper.Shipper
	shipperLock sync.Mutex
}

type ingesterFlusherCompat struct {
	*Ingester
}

func (i *ingesterFlusherCompat) Flush() {
	_, err := i.Ingester.Flush(context.TODO(), connect.NewRequest(&ingesterv1.FlushRequest{}))
	if err != nil {
		level.Error(i.Ingester.logger).Log("msg", "flush failed", "err", err)
	}
}

func New(cfg Config, logger log.Logger, reg prometheus.Registerer, firedb *firedb.FireDB, storageBucket fireobjstore.Bucket) (*Ingester, error) {
	i := &Ingester{
		cfg:    cfg,
		logger: logger,
		fireDB: firedb,
	}

	if storageBucket != nil {
		i.shipper = shipper.New(
			logger,
			reg,
			firedb,
			fireobjstore.BucketWithPrefix(storageBucket, "firedb"),
			block.IngesterSource,
			false,
			false,
		)
	}

	var err error
	i.lifecycler, err = ring.NewLifecycler(
		cfg.LifecyclerConfig,
		&ingesterFlusherCompat{i},
		"ingester",
		"ring",
		true,
		logger, prometheus.WrapRegistererWithPrefix("fire_", reg))
	if err != nil {
		return nil, err
	}

	i.lifecyclerWatcher = services.NewFailureWatcher()
	i.lifecyclerWatcher.WatchService(i.lifecycler)
	i.Service = services.NewBasicService(i.starting, i.running, i.stopping)
	return i, nil
}

func (i *Ingester) starting(ctx context.Context) error {
	// pass new context to lifecycler, so that it doesn't stop automatically when Ingester's service context is done
	err := i.lifecycler.StartAsync(context.Background())
	if err != nil {
		return err
	}

	err = i.lifecycler.AwaitRunning(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (i *Ingester) runShipper(ctx context.Context) {
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

func (i *Ingester) running(ctx context.Context) error {
	// run shipper periodically and at start-up
	shipperTicker := time.NewTicker(5 * time.Minute)
	defer shipperTicker.Stop()
	go func() {
		i.runShipper(ctx)
	}()

	for {
		select {

		case <-ctx.Done(): // wait until service is asked to stop
			return nil

		case err := <-i.lifecyclerWatcher.Chan(): // handle lifecycler errors
			return fmt.Errorf("lifecycler failed: %w", err)

		case <-shipperTicker.C: // run shipper loop
			i.runShipper(ctx)
		}
	}
}

func (i *Ingester) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	logger := log.With(i.logger, "tenant", tenantID)
	level.Debug(logger).Log("msg", "message received by ingester push")

	for _, series := range req.Msg.Series {
		for _, sample := range series.Samples {
			reader, err := gzip.NewReader(bytes.NewReader(sample.RawProfile))
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}

			p := profilev1.ProfileFromVTPool()
			if err := p.UnmarshalVT(data); err != nil {
				return nil, err
			}
			id, err := uuid.Parse(sample.ID)
			if err != nil {
				return nil, err
			}
			if err := i.fireDB.Head().Ingest(ctx, p, id, series.Labels...); err != nil {
				return nil, err
			}
			p.ReturnToVTPool()
		}
	}

	res := connect.NewResponse(&pushv1.PushResponse{})
	return res, nil
}

func (i *Ingester) stopping(_ error) error {
	return services.StopAndAwaitTerminated(context.Background(), i.lifecycler)
}

func (i *Ingester) Flush(ctx context.Context, req *connect.Request[ingesterv1.FlushRequest]) (*connect.Response[ingesterv1.FlushResponse], error) {
	if err := i.fireDB.Flush(ctx); err != nil {
		return nil, err
	}

	return connect.NewResponse(&ingesterv1.FlushResponse{}), nil
}

func (i *Ingester) TransferOut(ctx context.Context) error {
	return nil
}

// ReadinessHandler is used to indicate to k8s when the ingesters are ready for
// the addition removal of another ingester. Returns 204 when the ingester is
// ready, 500 otherwise.
func (i *Ingester) CheckReady(ctx context.Context) error {
	if s := i.State(); s != services.Running && s != services.Stopping {
		return fmt.Errorf("ingester not ready: %v", s)
	}
	return i.lifecycler.CheckReady(ctx)
}
