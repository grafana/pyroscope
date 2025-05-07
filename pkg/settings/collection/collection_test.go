package collection

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/testhelper"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/require"
)

type testCollection struct {
	*Collection
	bucketPath string
}

const (
	legacyStoreJSON  = `{"rules":[{"name":"my-valid-rule","generation":"2","ebpf":{"enabled": true},"services":[{"name":"valid-service","enabled":true},{"name":"second-valid-service"}],"lastUpdated":"1737625895123"}],"generation":"2"}`
	genericStoreJSON = `{"elements":[{"name":"my-valid-rule","generation":"2","ebpf":{"enabled": true},"services":[{"name":"valid-service","enabled":true},{"name":"second-valid-service"}],"lastUpdated":"1737625895123"}],"generation":"2"}`
)

func newTestCollection(t testing.TB) *testCollection {
	logger := log.NewNopLogger()
	if testing.Verbose() {
		logger = log.NewLogfmtLogger(os.Stderr)
	}
	cfg := Config{
		PyroscopeURL: "https://profiles-dev-007.grafana-dev.net/",
	}
	bucketPath := t.TempDir()
	bucket, err := filesystem.NewBucket(bucketPath)
	require.NoError(t, err)
	return &testCollection{
		Collection: New(cfg, bucket, logger),
		bucketPath: bucketPath,
	}
}

// TestCollection tests the packaced high level functionality in an integrated way.
func TestCollection(t *testing.T) {
	coll := newTestCollection(t)
	ctx := user.InjectOrgID(context.Background(), "user-a")
	ctxB := user.InjectOrgID(context.Background(), "user-b")

	t.Run("list empty collection", func(t *testing.T) {
		resp, err := coll.ListCollectionRules(ctx, connect.NewRequest(&settingsv1.ListCollectionRulesRequest{}))
		require.NoError(t, err)

		testhelper.EqualProto(t, &settingsv1.ListCollectionRulesResponse{
			Rules:      []*settingsv1.GetCollectionRuleResponse{},
			Generation: 0,
		}, resp.Msg)
	})

	t.Run("add ebpf rule", func(t *testing.T) {
		resp, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			Name: "my-ebpf-rule",
			Ebpf: &settingsv1.EBPFSettings{Enabled: true},
			Services: []*settingsv1.ServiceData{
				{Name: "service-a", Enabled: true},
				{Name: "service-b", Enabled: false},
			},
		}))
		require.NoError(t, err)
		actConfig := resp.Msg.Configuration
		expectConfig, err := os.ReadFile("./testdata/user-a-my-ebpf-config.alloy")
		require.NoError(t, err)
		require.Equal(t, string(expectConfig), actConfig)

		testhelper.EqualProto(t, []*settingsv1.ServiceData{
			{Name: "service-a", Enabled: true},
			{Name: "service-b", Enabled: false},
		}, resp.Msg.Services)
		require.Equal(t, true, resp.Msg.Ebpf.Enabled)
		require.Equal(t, false, resp.Msg.Java.Enabled)
	})

	t.Run("add java rule", func(t *testing.T) {
		resp, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			Name: "my-java-rule",
			Java: &settingsv1.JavaSettings{Enabled: true},
			Services: []*settingsv1.ServiceData{
				{Name: "service-a", Enabled: true},
				{Name: "service-b", Enabled: false},
				{Name: "service-hack.*\"", Enabled: false},
			},
		}))
		require.NoError(t, err)
		actConfig := resp.Msg.Configuration
		expectConfig, err := os.ReadFile("./testdata/user-a-my-java-config.alloy")
		require.NoError(t, err)
		require.Equal(t, string(expectConfig), actConfig)

		testhelper.EqualProto(t, []*settingsv1.ServiceData{
			{Name: "service-a", Enabled: true},
			{Name: "service-b", Enabled: false},
			{Name: "service-hack.*\"", Enabled: false},
		}, resp.Msg.Services)
		require.Equal(t, false, resp.Msg.Ebpf.Enabled)
		require.Equal(t, true, resp.Msg.Java.Enabled)
	})

	t.Run("update java one rule to also have ebpf", func(t *testing.T) {
		resp, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			Name: "my-java-rule",
			Ebpf: &settingsv1.EBPFSettings{Enabled: true},
			Java: &settingsv1.JavaSettings{Enabled: true},
			Services: []*settingsv1.ServiceData{
				{Name: "service-a", Enabled: true},
				{Name: "service-b", Enabled: false},
				{Name: "service-hack.*\"", Enabled: false},
			},
		}))
		require.NoError(t, err)
		actConfig := resp.Msg.Configuration
		expectConfig, err := os.ReadFile("./testdata/user-a-my-java-config-v2.alloy")
		require.NoError(t, err)
		require.Equal(t, string(expectConfig), actConfig)

		testhelper.EqualProto(t, []*settingsv1.ServiceData{
			{Name: "service-a", Enabled: true},
			{Name: "service-b", Enabled: false},
			{Name: "service-hack.*\"", Enabled: false},
		}, resp.Msg.Services)
		require.Equal(t, true, resp.Msg.Ebpf.Enabled)
		require.Equal(t, true, resp.Msg.Java.Enabled)
	})

	t.Run("delete java one", func(t *testing.T) {
		_, err := coll.DeleteCollectionRule(ctx, connect.NewRequest(&settingsv1.DeleteCollectionRuleRequest{
			Name: "my-java-rule",
		}))
		require.NoError(t, err)

		resp, err := coll.ListCollectionRules(ctx, connect.NewRequest(&settingsv1.ListCollectionRulesRequest{}))
		require.NoError(t, err)

		require.Equal(t, 1, len(resp.Msg.Rules))
		require.Equal(t, "my-ebpf-rule", resp.Msg.Rules[0].Name)
	})

	t.Run("ensure tenant user-b has no rules at all", func(t *testing.T) {
		resp, err := coll.ListCollectionRules(ctxB, connect.NewRequest(&settingsv1.ListCollectionRulesRequest{}))
		require.NoError(t, err)

		testhelper.EqualProto(t, &settingsv1.ListCollectionRulesResponse{
			Rules:      []*settingsv1.GetCollectionRuleResponse{},
			Generation: 0,
		}, resp.Msg)
	})
}

func TestUpsertCollectionRule(t *testing.T) {
	coll := newTestCollection(t)
	ctx := user.InjectOrgID(context.Background(), "user-a")

	// fix time
	timeNow = func() time.Time {
		return time.Unix(1737625895, 123456789)
	}
	defer func() {
		timeNow = time.Now
	}()

	t.Run("valid", func(t *testing.T) {
		_, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			Name: "my-valid-rule",
			Services: []*settingsv1.ServiceData{
				{Name: "valid-service", Enabled: true},
			},
		}))
		require.NoError(t, err)
	})
	t.Run("update to valid rule", func(t *testing.T) {
		observedGeneration := int64(1)
		_, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			ObservedGeneration: &observedGeneration,
			Name:               "my-valid-rule",
			Ebpf:               &settingsv1.EBPFSettings{Enabled: true},
			Services: []*settingsv1.ServiceData{
				{Name: "valid-service", Enabled: true},
				{Name: "second-valid-service", Enabled: false},
			},
		}))
		require.NoError(t, err)
		data, err := os.ReadFile(filepath.Join(coll.bucketPath, "user-a/settings/collection.v1.json"))
		require.NoError(t, err)
		require.JSONEq(
			t,
			genericStoreJSON,
			string(data),
		)

	})
	t.Run("conflicting update", func(t *testing.T) {
		observedGeneration := int64(1)
		_, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			ObservedGeneration: &observedGeneration,
			Name:               "my-valid-rule",
			Ebpf:               &settingsv1.EBPFSettings{Enabled: true},
			Services:           []*settingsv1.ServiceData{},
		}))
		require.ErrorContains(t, err, "already_exists: Conflicting update, please try again")
	})
	t.Run("invalid rule name", func(t *testing.T) {
		_, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			Name: "my-Invalid-rule",
			Services: []*settingsv1.ServiceData{
				{Name: "valid-service", Enabled: true},
				{Name: "second-valid-service", Enabled: false},
			},
		}))
		require.ErrorContains(t, err, "invalid_argument: invalid name 'my-Invalid-rule', must match ^[a-z0-9-]+$")
	})
	t.Run("invalid service name", func(t *testing.T) {
		_, err := coll.UpsertCollectionRule(ctx, connect.NewRequest(&settingsv1.UpsertCollectionRuleRequest{
			Name: "my-valid-rule",
			Services: []*settingsv1.ServiceData{
				{Name: "valid-service", Enabled: true},
				{Name: "invalid-service`", Enabled: false},
			},
		}))
		require.ErrorContains(t, err, "invalid_argument: invalid service name 'invalid-service`', must not contain '`'")
	})

}

func TestListCollectionRules(t *testing.T) {
	coll := newTestCollection(t)
	ctx := user.InjectOrgID(context.Background(), "user-a")

	t.Run("list from legacy storage format", func(t *testing.T) {
		storePath := filepath.Join(coll.bucketPath, "user-a/settings/collection.v1.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o755))
		require.NoError(t, os.WriteFile(
			storePath,
			[]byte(legacyStoreJSON),
			0o644,
		))

		resp, err := coll.ListCollectionRules(ctx, connect.NewRequest(&settingsv1.ListCollectionRulesRequest{}))
		require.NoError(t, err)

		require.Len(t, resp.Msg.Rules, 1)

		// reset config
		resp.Msg.Rules[0].Configuration = ""

		testhelper.EqualProto(t, &settingsv1.ListCollectionRulesResponse{
			Rules: []*settingsv1.GetCollectionRuleResponse{
				{
					Name: "my-valid-rule",
					Services: []*settingsv1.ServiceData{
						{Name: "valid-service", Enabled: true},
						{Name: "second-valid-service", Enabled: false},
					},
					Ebpf:        &settingsv1.EBPFSettings{Enabled: true},
					Java:        &settingsv1.JavaSettings{Enabled: false},
					Generation:  2,
					LastUpdated: 1737625895123,
				},
			},
			Generation: 2,
		}, resp.Msg)

	})

	t.Run("list from generic store format", func(t *testing.T) {
		storePath := filepath.Join(coll.bucketPath, "user-a/settings/collection.v1.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o755))
		require.NoError(t, os.WriteFile(
			storePath,
			[]byte(genericStoreJSON),
			0o644,
		))

		resp, err := coll.ListCollectionRules(ctx, connect.NewRequest(&settingsv1.ListCollectionRulesRequest{}))
		require.NoError(t, err)

		require.Len(t, resp.Msg.Rules, 1)

		// reset config
		resp.Msg.Rules[0].Configuration = ""

		testhelper.EqualProto(t, &settingsv1.ListCollectionRulesResponse{
			Rules: []*settingsv1.GetCollectionRuleResponse{
				{
					Name: "my-valid-rule",
					Services: []*settingsv1.ServiceData{
						{Name: "valid-service", Enabled: true},
						{Name: "second-valid-service", Enabled: false},
					},
					Ebpf:        &settingsv1.EBPFSettings{Enabled: true},
					Java:        &settingsv1.JavaSettings{Enabled: false},
					Generation:  2,
					LastUpdated: 1737625895123,
				},
			},
			Generation: 2,
		}, resp.Msg)

	})
}
