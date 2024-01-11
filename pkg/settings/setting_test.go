package settings

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/tenant"
)

func TestTenantSettings_Get(t *testing.T) {
	t.Run("get a setting", func(t *testing.T) {
		const tenantID = "1234"
		wantSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1",
			ModifiedAt: 100,
		}

		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{
			tenantID: {
				wantSetting,
			},
		})
		defer cleanup()

		ctx := tenant.InjectTenantID(context.Background(), tenantID)
		req := &connect.Request[settingsv1.GetSettingsRequest]{}

		got, err := ts.Get(ctx, req)
		require.NoError(t, err)

		want := &settingsv1.GetSettingsResponse{
			Settings: []*settingsv1.Setting{wantSetting},
		}
		require.Equal(t, want, got.Msg)
	})

	t.Run("missing tenant id", func(t *testing.T) {
		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{})
		defer cleanup()

		ctx := context.Background()
		req := &connect.Request[settingsv1.GetSettingsRequest]{}

		_, err := ts.Get(ctx, req)
		require.EqualError(t, err, "invalid_argument: no org id")
	})

	t.Run("settings store returns error", func(t *testing.T) {
		store := &fakeStore{}
		wantErr := fmt.Errorf("settings store failed")

		// Get method fails once.
		store.On("Get", mock.Anything, mock.Anything).
			Return(nil, wantErr).
			Once()

		ts := &TenantSettings{
			store:  store,
			logger: log.NewNopLogger(),
		}

		ctx := tenant.InjectTenantID(context.Background(), "1234")
		req := &connect.Request[settingsv1.GetSettingsRequest]{}

		_, err := ts.Get(ctx, req)
		require.EqualError(t, err, fmt.Sprintf("internal: %s", wantErr))
	})
}

func TestTenantSettings_Set(t *testing.T) {
	t.Run("set a new setting", func(t *testing.T) {
		const tenantID = "1234"
		wantSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1",
			ModifiedAt: 100,
		}

		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{})
		defer cleanup()

		ctx := tenant.InjectTenantID(context.Background(), tenantID)
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: wantSetting,
			},
		}

		got, err := ts.Set(ctx, req)
		require.NoError(t, err)

		want := &settingsv1.SetSettingsResponse{
			Setting: wantSetting,
		}
		require.Equal(t, want, got.Msg)
	})

	t.Run("set a new setting without a timestamp", func(t *testing.T) {
		const tenantID = "1234"
		wantSetting := &settingsv1.Setting{
			Name:  "key1",
			Value: "val1",
		}

		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{})
		defer cleanup()

		ctx := tenant.InjectTenantID(context.Background(), tenantID)
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: wantSetting,
			},
		}

		got, err := ts.Set(ctx, req)
		require.NoError(t, err)

		want := &settingsv1.SetSettingsResponse{
			Setting: wantSetting,
		}
		require.Equal(t, want.Setting.Name, got.Msg.Setting.Name)
		require.Equal(t, want.Setting.Value, got.Msg.Setting.Value)
		require.NotZero(t, got.Msg.Setting.ModifiedAt, "ModifiedAt value did not get set")
	})

	t.Run("update a setting", func(t *testing.T) {
		const tenantID = "1234"
		initialSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1",
			ModifiedAt: 100,
		}
		wantSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1 (new)",
			ModifiedAt: 101,
		}

		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{
			tenantID: {
				initialSetting,
			},
		})
		defer cleanup()

		ctx := tenant.InjectTenantID(context.Background(), tenantID)
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: wantSetting,
			},
		}

		got, err := ts.Set(ctx, req)
		require.NoError(t, err)

		want := &settingsv1.SetSettingsResponse{
			Setting: wantSetting,
		}
		require.Equal(t, want, got.Msg)
	})

	t.Run("missing tenant id", func(t *testing.T) {
		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{})
		defer cleanup()

		ctx := context.Background()
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: &settingsv1.Setting{},
			},
		}

		_, err := ts.Set(ctx, req)
		require.EqualError(t, err, "invalid_argument: no org id")
	})

	t.Run("missing setting values", func(t *testing.T) {
		const tenantID = "1234"

		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{})
		defer cleanup()

		ctx := tenant.InjectTenantID(context.Background(), tenantID)
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: nil, // Purposely empty
			},
		}

		_, err := ts.Set(ctx, req)
		require.EqualError(t, err, "invalid_argument: no setting values provided")
	})

	t.Run("already exists", func(t *testing.T) {
		const tenantID = "1234"
		initialSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1",
			ModifiedAt: 100,
		}
		wantSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1 (new)",
			ModifiedAt: 99, // Timestamp older than most current.
		}

		ts, cleanup := newTestTenantSettings(t, map[string][]*settingsv1.Setting{
			tenantID: {
				initialSetting,
			},
		})
		defer cleanup()

		ctx := tenant.InjectTenantID(context.Background(), tenantID)
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: wantSetting,
			},
		}

		_, err := ts.Set(ctx, req)
		require.EqualError(t, err, "already_exists: failed to update key1: newer update already written")
	})

	t.Run("settings store returns error", func(t *testing.T) {
		store := &fakeStore{}
		wantErr := fmt.Errorf("settings store failed")

		// Get method fails once.
		store.On("Set", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, wantErr).
			Once()

		ts := &TenantSettings{
			store:  store,
			logger: log.NewNopLogger(),
		}

		ctx := tenant.InjectTenantID(context.Background(), "1234")
		req := &connect.Request[settingsv1.SetSettingsRequest]{
			Msg: &settingsv1.SetSettingsRequest{
				Setting: &settingsv1.Setting{
					Name:       "key1",
					Value:      "val1",
					ModifiedAt: 100,
				},
			},
		}

		_, err := ts.Set(ctx, req)
		require.EqualError(t, err, fmt.Sprintf("internal: %s", wantErr))
	})
}

func newTestTenantSettings(t *testing.T, initial map[string][]*settingsv1.Setting) (*TenantSettings, func()) {
	t.Helper()

	store, err := NewMemoryStore()
	require.NoError(t, err)

	for tenant, settings := range initial {
		for _, setting := range settings {
			_, err = store.Set(context.Background(), tenant, setting)
			require.NoError(t, err)
		}
	}

	ts := &TenantSettings{
		store:  store,
		logger: log.NewNopLogger(),
	}

	cleanupFn := func() {
		ts.store.Close()
	}

	return ts, cleanupFn
}

type fakeStore struct {
	mock.Mock
}

func (s *fakeStore) Get(ctx context.Context, tenantID string) ([]*settingsv1.Setting, error) {
	args := s.Called(ctx, tenantID)
	if args.Get(0) == nil {
		args[0] = []*settingsv1.Setting{}
	}

	return args.Get(0).([]*settingsv1.Setting), args.Error(1)
}

func (s *fakeStore) Set(ctx context.Context, tenantID string, setting *settingsv1.Setting) (*settingsv1.Setting, error) {
	args := s.Called(ctx, tenantID, setting)
	if args.Get(0) == nil {
		args[0] = &settingsv1.Setting{}
	}

	return args.Get(0).(*settingsv1.Setting), args.Error(1)
}

func (s *fakeStore) Flush(ctx context.Context) error {
	args := s.Called(ctx)
	return args.Error(0)
}

func (s *fakeStore) Close() error {
	args := s.Called()
	return args.Error(0)
}
