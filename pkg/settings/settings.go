package settings

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	"github.com/thanos-io/objstore"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	"github.com/grafana/pyroscope/pkg/settings/recording"
	"github.com/grafana/pyroscope/pkg/validation"
)

type Config struct {
	Recording recording.Config `yaml:"recording_rules"`
}

func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.Recording.RegisterFlags(fs)
}

func (cfg *Config) Validate() error {
	return errors.Join(
		cfg.Recording.Validate(),
	)
}

var _ settingsv1connect.SettingsServiceHandler = (*TenantSettings)(nil)

func New(cfg Config, bucket objstore.Bucket, logger log.Logger, overrides *validation.Overrides) (*TenantSettings, error) {
	if bucket == nil {
		bucket = objstore.NewInMemBucket()
		level.Warn(logger).Log("msg", "using in-memory settings store, changes will be lost after shutdown")
	}

	ts := &TenantSettings{
		RecordingRulesServiceHandler: &settingsv1connect.UnimplementedRecordingRulesServiceHandler{},
		store:                        newBucketStore(bucket),
		logger:                       logger,
	}

	if cfg.Recording.Enabled {
		ts.RecordingRulesServiceHandler = recording.New(bucket, logger, overrides)
	}

	ts.Service = services.NewBasicService(ts.starting, ts.running, ts.stopping)

	return ts, nil
}

type TenantSettings struct {
	services.Service
	settingsv1connect.RecordingRulesServiceHandler

	store  store
	logger log.Logger
}

func (ts *TenantSettings) starting(ctx context.Context) error {
	return nil
}

func (ts *TenantSettings) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (ts *TenantSettings) stopping(_ error) error {
	err := ts.store.Close()
	if err != nil {
		return err
	}
	return nil
}

func (ts *TenantSettings) Get(
	ctx context.Context,
	req *connect.Request[settingsv1.GetSettingsRequest],
) (*connect.Response[settingsv1.GetSettingsResponse], error) {
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

func (ts *TenantSettings) Set(
	ctx context.Context,
	req *connect.Request[settingsv1.SetSettingsRequest],
) (*connect.Response[settingsv1.SetSettingsResponse], error) {
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

func (ts *TenantSettings) Delete(ctx context.Context, req *connect.Request[settingsv1.DeleteSettingsRequest]) (*connect.Response[settingsv1.DeleteSettingsResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if req.Msg == nil || req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no setting name provided"))
	}

	modifiedAt := time.Now().UnixMilli()
	err = ts.store.Delete(ctx, tenantID, req.Msg.Name, modifiedAt)
	if err != nil {
		if errors.Is(err, oldSettingErr) {
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.DeleteSettingsResponse{}), nil
}
