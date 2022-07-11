package ingester

/*
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
*/

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
