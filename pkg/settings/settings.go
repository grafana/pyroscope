package settings

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	"github.com/pkg/errors"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/settings/collection"
)

func New(bucket objstore.Bucket, logger log.Logger) (*TenantSettings, error) {
	if bucket == nil {
		bucket = objstore.NewInMemBucket()
		level.Warn(logger).Log("msg", "using in-memory settings store, changes will be lost after shutdown")
	}

	ts := &TenantSettings{
		store:      newBucketStore(bucket),
		logger:     logger,
		collection: collection.New(collection.Config{}, bucket, logger),
	}

	ts.Service = services.NewBasicService(ts.starting, ts.running, ts.stopping)

	return ts, nil
}

type TenantSettings struct {
	services.Service

	store  store
	logger log.Logger

	collection *collection.Collection
}

func (ts *TenantSettings) starting(ctx context.Context) error {
	return nil
}

func (ts *TenantSettings) HandleCollectionSettings(role collection.Role) http.Handler {
	return ts.collection.HandleSettings(role)
}

func (ts *TenantSettings) running(ctx context.Context) error {
	ticker := time.NewTicker(24 * time.Hour)
	done := false

	for !done {
		select {
		case <-ticker.C:
			err := ts.store.Flush(ctx)
			if err != nil {
				level.Warn(ts.logger).Log(
					"msg", "failed to refresh tenant settings",
					"err", err,
				)
			}
		case <-ctx.Done():
			ticker.Stop()
			done = true
		}
	}

	return nil
}

func (ts *TenantSettings) stopping(_ error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ts.collection.Stop(ctx)

	err := ts.store.Flush(ctx)
	if err != nil {
		return err
	}

	err = ts.store.Close()
	if err != nil {
		return err
	}
	return nil
}

func (ts *TenantSettings) Get(ctx context.Context, req *connect.Request[settingsv1.GetSettingsRequest]) (*connect.Response[settingsv1.GetSettingsResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	settings, err := ts.store.Get(ctx, tenantID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.GetSettingsResponse{
		Settings: settings,
	}), nil
}

func (ts *TenantSettings) Set(ctx context.Context, req *connect.Request[settingsv1.SetSettingsRequest]) (*connect.Response[settingsv1.SetSettingsResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if req.Msg == nil || req.Msg.Setting == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no setting values provided"))
	}

	if req.Msg.Setting.ModifiedAt <= 0 {
		req.Msg.Setting.ModifiedAt = time.Now().UnixMilli()
	}

	setting, err := ts.store.Set(ctx, tenantID, req.Msg.Setting)
	if err != nil {
		if errors.Is(err, oldSettingErr) {
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.SetSettingsResponse{
		Setting: setting,
	}), nil
}
