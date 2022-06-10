package ingester

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime/pprof"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/fire/pkg/gen/ingester/v1/ingestv1connect"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/profilestore"
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

func Test_ConnectPush(t *testing.T) {
	cfg := defaultIngesterTestConfig(t)
	logger := log.NewLogfmtLogger(os.Stdout)

	profileStore, err := profilestore.New(logger, nil, trace.NewNoopTracerProvider())
	require.NoError(t, err)

	mux := http.NewServeMux()
	d, err := New(cfg, log.NewLogfmtLogger(os.Stdout), nil, profileStore)
	require.NoError(t, err)

	mux.Handle(ingestv1connect.NewIngesterHandler(d))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := ingestv1connect.NewIngesterClient(http.DefaultClient, s.URL)

	resp, err := client.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*pushv1.LabelPair{
					{Name: "__name__", Value: "my_own_profile"},
					{Name: "cluster", Value: "us-central1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: testProfile(t),
					},
				},
			},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.NoError(
		t,
		profileStore.Close(),
	)
}

func testProfile(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}

func Test_ParseQuery(t *testing.T) {
	q := url.Values{
		"query": []string{`memory:alloc_space:bytes:space:bytes{foo="bar",bar=~"buzz"}`},
		"from":  []string{"now-6h"},
		"until": []string{"now"},
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost/render/render?%s", q.Encode()), nil)
	require.NoError(t, err)

	expr, err := parseQuery(req)
	require.NoError(t, err)

	require.NotNil(t, expr)
}

func Test_selectMerge(t *testing.T) {
	cfg := defaultIngesterTestConfig(t)
	profileStore, err := profilestore.New(log.NewNopLogger(), nil, trace.NewNoopTracerProvider())
	require.NoError(t, err)
	testPprof := testProfile(t)
	d, err := New(cfg, log.NewNopLogger(), nil, profileStore)
	require.NoError(t, err)
	resp, err := d.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*pushv1.LabelPair{
					{Name: "__name__", Value: "memory"},
					// {Name: "cluster", Value: "us-central1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: testPprof,
					},
				},
			},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	os.WriteFile("/Users/cyriltovena/work/fire/test.pprof", testPprof, 0o644)

	flame, err := d.selectMerge(context.Background(), selectMerge{
		query: `memory:inuse_space:bytes:space:bytes{}`,
		start: 0,
		end:   int64(model.Now()),
	})
	require.NoError(t, err)

	spew.Config.Dump(flame)

	require.NoError(
		t,
		profileStore.Close(),
	)
}
