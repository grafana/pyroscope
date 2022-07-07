package ingester

import (
	"bytes"
	"compress/gzip"
	"io"
	"runtime/pprof"
	"testing"

	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/require"
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

/*
func Test_ConnectPush(t *testing.T) {
	cfg := defaultIngesterTestConfig(t)
	logger := log.NewLogfmtLogger(os.Stdout)

	profileStore, err := profilestore.New(logger, nil, trace.NewNoopTracerProvider(), defaultProfileStoreTestConfig(t))
	require.NoError(t, err)

	mux := http.NewServeMux()
	d, err := New(cfg, log.NewLogfmtLogger(os.Stdout), nil, profileStore)
	require.NoError(t, err)

	mux.Handle(ingesterv1connect.NewIngesterServiceHandler(d))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := ingesterv1connect.NewIngesterServiceClient(http.DefaultClient, s.URL)

	rawProfile := testProfile(t)
	resp, err := client.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*commonv1.LabelPair{
					{Name: "__name__", Value: "my_own_profile"},
					{Name: "cluster", Value: "us-central1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: rawProfile,
					},
				},
			},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
	ingestedSamples := countNonZeroValues(parseRawProfile(t, bytes.NewBuffer(rawProfile)))

	profileStore.Table().Sync()
	var queriedSamples int64
	require.NoError(t, profileStore.Table().View(func(tx uint64) error {
		return profileStore.Table().Iterator(context.Background(), tx, memory.NewGoAllocator(), nil, nil, nil, func(ar arrow.Record) error {
			t.Log(ar)

			queriedSamples += ar.NumRows()

			return nil
		})
	}))

	require.Equal(t, ingestedSamples, queriedSamples, "expected to query all ingested samples")

	require.NoError(t, profileStore.Table().RotateBlock())

	require.NoError(
		t,
		profileStore.Close(),
	)
}
*/

// This counts all sample values, where at least a single value in a sample is non-zero
func countNonZeroValues(p *profile.Profile) int64 {
	var count int64
	for _, s := range p.Sample {
		for _, v := range s.Value {
			if v != 0 {
				count++
				continue
			}
		}
	}
	return count
}

func parseRawProfile(t testing.TB, reader io.Reader) *profile.Profile {
	gzReader, err := gzip.NewReader(reader)
	require.NoError(t, err, "failed creating gzip reader")

	p, err := profile.Parse(gzReader)
	require.NoError(t, err, "failed parsing profile")

	return p
}

func testProfile(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}
