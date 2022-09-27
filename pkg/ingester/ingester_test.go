package ingester

import (
	"bytes"
	"context"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/grafana/fire/pkg/firedb"
	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	firemodel "github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/objstore/client"
	"github.com/grafana/fire/pkg/tenant"
)

func defaultIngesterTestConfig(t testing.TB) Config {
	kvClient, err := kv.NewClient(kv.Config{Store: "inmemory"}, ring.GetCodec(), nil, log.NewNopLogger())
	require.NoError(t, err)
	cfg := Config{}
	flagext.DefaultValues(&cfg)
	cfg.LifecyclerConfig.RingConfig.KVStore.Mock = kvClient
	cfg.LifecyclerConfig.NumTokens = 1
	cfg.LifecyclerConfig.ListenPort = 0
	cfg.LifecyclerConfig.Addr = "localhost"
	cfg.LifecyclerConfig.ID = "localhost"
	cfg.LifecyclerConfig.FinalSleep = 0
	cfg.LifecyclerConfig.MinReadyDuration = 0
	return cfg
}

func testProfile(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}

func Test_MultitenantReadWrite(t *testing.T) {
	dbPath := t.TempDir()
	logger := log.NewJSONLogger(os.Stdout)
	reg := prometheus.NewRegistry()

	fs, err := client.NewBucket(logger, []byte(`
type: FILESYSTEM
config:
    directory: "`+dbPath+`"
prefix: ""`), reg, "storage")
	require.NoError(t, err)

	ing, err := New(defaultIngesterTestConfig(t), firedb.Config{
		DataPath:         dbPath,
		MaxBlockDuration: 30 * time.Hour,
	}, logger, reg, fs)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), ing))

	req := &connect.Request[pushv1.PushRequest]{
		Msg: &pushv1.PushRequest{
			Series: []*pushv1.RawProfileSeries{
				{
					Samples: []*pushv1.RawSample{
						{
							ID:         uuid.NewString(),
							RawProfile: testProfile(t),
						},
					},
				},
			},
		},
	}
	req.Msg.Series[0].Labels = firemodel.LabelsFromStrings("foo", "bar")
	_, err = ing.Push(tenant.InjectTenantID(context.Background(), "foo"), req)
	require.NoError(t, err)

	req.Msg.Series[0].Labels = firemodel.LabelsFromStrings("buzz", "bazz")
	_, err = ing.Push(tenant.InjectTenantID(context.Background(), "buzz"), req)
	require.NoError(t, err)

	labelNames, err := ing.LabelNames(tenant.InjectTenantID(context.Background(), "foo"), connect.NewRequest(&ingesterv1.LabelNamesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"__period_type__", "__period_unit__", "__profile_type__", "__type__", "__unit__", "foo"}, labelNames.Msg.Names)

	labelNames, err = ing.LabelNames(tenant.InjectTenantID(context.Background(), "buzz"), connect.NewRequest(&ingesterv1.LabelNamesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"__period_type__", "__period_unit__", "__profile_type__", "__type__", "__unit__", "buzz"}, labelNames.Msg.Names)

	labelsValues, err := ing.LabelValues(tenant.InjectTenantID(context.Background(), "foo"), connect.NewRequest(&ingesterv1.LabelValuesRequest{Name: "foo"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar"}, labelsValues.Msg.Names)

	labelsValues, err = ing.LabelValues(tenant.InjectTenantID(context.Background(), "buzz"), connect.NewRequest(&ingesterv1.LabelValuesRequest{Name: "buzz"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bazz"}, labelsValues.Msg.Names)

	require.NoError(t, services.StopAndAwaitTerminated(context.Background(), ing))
}
