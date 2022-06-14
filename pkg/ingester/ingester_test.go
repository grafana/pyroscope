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
	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
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
	// testPprof := testProfile(t)
	testPprof, err := os.ReadFile("/Users/cyril/work/fire/test.pprof")
	if err != nil {
		panic(err)
	}
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
	// if err := os.WriteFile("/Users/cyril/work/fire/test.pprof", testPprof, 0o644); err != nil {
	// 	panic(err)
	// }

	// flame, err := d.selectMerge(context.Background(), selectMerge{
	// 	query: `memory:inuse_space:bytes:space:bytes{}`,
	// 	start: 0,
	// 	end:   int64(model.Now()),
	// })
	// require.NoError(t, err)

	// spew.Config.Dump(flame)
	http.ListenAndServe(":8090", http.HandlerFunc(d.RenderHandler))

	require.NoError(
		t,
		profileStore.Close(),
	)
}

func Test_toFlamebearer(t *testing.T) {
	require.Equal(t, flamebearer.FlamebearerV1{
		Names: []string{"total", "a", "c", "d", "b", "e"},
		Levels: [][]int{
			{0, 4, 0, 0},
			{0, 4, 0, 1},
			{0, 1, 0, 4, 0, 3, 2, 2},
			{0, 1, 1, 5, 2, 1, 1, 3},
		},
		NumTicks: 4,
		MaxSelf:  2,
	}, stacksToTree([]stack{
		{
			locations: []location{
				{
					function: "e",
				},
				{
					function: "b",
				},
				{
					function: "a",
				},
			},
			value: 1,
		},
		{
			locations: []location{
				{
					function: "c",
				},
				{
					function: "a",
				},
			},
			value: 2,
		},
		{
			locations: []location{
				{
					function: "d",
				},
				{
					function: "c",
				},
				{
					function: "a",
				},
			},
			value: 1,
		},
	}).toFlamebearer())
}

func Test_Tree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stacks   []stack
		expected func() *tree
	}{
		{
			"empty",
			[]stack{},
			func() *tree { return &tree{} },
		},
		{
			"double node single stack",
			[]stack{
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
			},
			func() *tree {
				tr := NewTree()
				tr.Add("bar", 0, 2).Add("buz", 2, 2)
				return tr
			},
		},
		{
			"double node double stack",
			[]stack{
				{
					locations: []location{
						{
							function: "blip",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "blap",
						},
						{
							function: "blop",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 2,
				},
			},
			func() *tree {
				tr := NewTree()
				buz := tr.Add("bar", 0, 3).Add("buz", 0, 3)
				buz.Add("blip", 1, 1)
				buz.Add("blop", 0, 2).Add("blap", 2, 2)
				return tr
			},
		},
		{
			"multiple stacks and duplicates nodes",
			[]stack{
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "buz",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "foo",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 1,
				},
				{
					locations: []location{
						{
							function: "blop",
						},
						{
							function: "buz",
						},
						{
							function: "bar",
						},
					},
					value: 2,
				},
				{
					locations: []location{
						{
							function: "blip",
						},
						{
							function: "bar",
						},
					},
					value: 4,
				},
			},
			func() *tree {
				tr := NewTree()

				bar := tr.Add("bar", 0, 9)

				buz := bar.Add("buz", 2, 5)
				buz.Add("foo", 1, 1)
				buz.Add("blop", 2, 2)
				bar.Add("blip", 4, 4)

				tr.Add("buz", 1, 1)
				return tr
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected := tc.expected()
			tr := stacksToTree(tc.stacks)
			require.Equal(t, tr, expected, "tree should be equal got:%s\n expected:%s\n", tr.String(), expected)
		})
	}
}
