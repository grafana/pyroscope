package phlaredb

import (
	"context"
	"math"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func TestCreateLocalDir(t *testing.T) {
	ctx := testContext(t)
	dataPath := contextDataDir(ctx)
	localFile := dataPath + "/local"
	require.NoError(t, os.WriteFile(localFile, []byte("d"), 0o644))
	_, err := New(testContext(t), Config{
		DataPath:         dataPath,
		MaxBlockDuration: 30 * time.Minute,
	}, NoLimit, ctx.localBucketClient)
	require.Error(t, err)
	require.NoError(t, os.Remove(localFile))
	_, err = New(ctx, Config{
		DataPath:         dataPath,
		MaxBlockDuration: 30 * time.Minute,
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)
}

var cpuProfileGenerator = func(tsNano int64, t testing.TB) (*googlev1.Profile, string) {
	p := parseProfile(t, "testdata/profile")
	p.TimeNanos = tsNano
	return p, "process_cpu"
}

func ingestProfiles(b testing.TB, db *PhlareDB, generator func(tsNano int64, t testing.TB) (*googlev1.Profile, string), from, to int64, step time.Duration, externalLabels ...*typesv1.LabelPair) {
	b.Helper()
	for i := from; i <= to; i += int64(step) {
		p, name := generator(i, b)
		require.NoError(b, db.Ingest(
			context.Background(), p, uuid.New(), append(externalLabels, &typesv1.LabelPair{Name: model.MetricNameLabel, Value: name})...))
	}
}

type fakeBidiServerMergeProfilesStacktraces struct {
	profilesSent []*ingestv1.ProfileSets
	keep         [][]bool
	t            *testing.T
}

func (f *fakeBidiServerMergeProfilesStacktraces) Send(resp *ingestv1.MergeProfilesStacktracesResponse) error {
	f.profilesSent = append(f.profilesSent, testhelper.CloneProto(f.t, resp.SelectedProfiles))
	return nil
}

func (f *fakeBidiServerMergeProfilesStacktraces) Receive() (*ingestv1.MergeProfilesStacktracesRequest, error) {
	res := &ingestv1.MergeProfilesStacktracesRequest{
		Profiles: f.keep[0],
	}
	f.keep = f.keep[1:]
	return res, nil
}

func (q Queriers) ingesterClient() (ingesterv1connect.IngesterServiceClient, func()) {
	mux := http.NewServeMux()
	mux.Handle(ingesterv1connect.NewIngesterServiceHandler(&ingesterHandlerPhlareDB{q}))
	serv := testhelper.NewInMemoryServer(mux)

	var httpClient *http.Client = serv.Client()

	client := ingesterv1connect.NewIngesterServiceClient(
		httpClient, serv.URL(),
	)

	return client, serv.Close
}

type ingesterHandlerPhlareDB struct {
	Queriers
	// *PhlareDB
}

func (i *ingesterHandlerPhlareDB) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return MergeProfilesStacktraces(ctx, stream, i.forTimeRange)
}

func (i *ingesterHandlerPhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return MergeProfilesLabels(ctx, stream, i.forTimeRange)
}

func (i *ingesterHandlerPhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return MergeProfilesPprof(ctx, stream, i.forTimeRange)
}

func (i *ingesterHandlerPhlareDB) MergeSpanProfile(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeSpanProfileRequest, ingestv1.MergeSpanProfileResponse]) error {
	return MergeSpanProfile(ctx, stream, i.forTimeRange)
}

func (i *ingesterHandlerPhlareDB) Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) Series(context.Context, *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) Flush(context.Context, *connect.Request[ingestv1.FlushRequest]) (*connect.Response[ingestv1.FlushResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) BlockMetadata(context.Context, *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) GetProfileStats(context.Context, *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) GetBlockStats(context.Context, *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error) {
	return nil, errors.New("not implemented")
}

func TestMergeProfilesStacktraces(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// ingest some sample data
	var (
		ctx     = testContext(t)
		testDir = contextDataDir(ctx)
		end     = time.Unix(0, int64(time.Hour))
		start   = end.Add(-time.Minute)
		step    = 15 * time.Second
	)

	db, err := New(ctx, Config{
		DataPath:         testDir,
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	// create client
	client, cleanup := db.queriers().ingesterClient()
	defer cleanup()

	t.Run("request the one existing series", func(t *testing.T) {
		bidi := client.MergeProfilesStacktraces(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))

		resp, err := bidi.Receive()
		require.NoError(t, err)
		require.Nil(t, resp.Result)
		require.Len(t, resp.SelectedProfiles.LabelsSets, 1)
		require.Len(t, resp.SelectedProfiles.Profiles, 5)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Profiles: []bool{true},
		}))

		// expect empty response
		resp, err = bidi.Receive()
		require.NoError(t, err)
		require.Nil(t, resp.Result)

		// received result
		resp, err = bidi.Receive()
		require.NoError(t, err)
		require.NotNil(t, resp.Result)

		at, err := phlaremodel.UnmarshalTree(resp.Result.TreeBytes)
		require.NoError(t, err)
		require.Equal(t, int64(500000000), at.Total())
	})

	t.Run("request non existing series", func(t *testing.T) {
		bidi := client.MergeProfilesStacktraces(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="not-my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))

		// expect empty resp to signal it is finished
		resp, err := bidi.Receive()
		require.NoError(t, err)
		require.Nil(t, resp.Result)
		require.Nil(t, resp.SelectedProfiles)

		// still receiving a result
		resp, err = bidi.Receive()
		require.NoError(t, err)
		require.NotNil(t, resp.Result)
		require.Len(t, resp.Result.Stacktraces, 0)
		require.Len(t, resp.Result.FunctionNames, 0)
		require.Nil(t, resp.SelectedProfiles)
	})

	t.Run("empty request fails", func(t *testing.T) {
		bidi := client.MergeProfilesStacktraces(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{}))

		_, err := bidi.Receive()
		require.EqualError(t, err, "invalid_argument: missing initial select request")
	})

	t.Run("test cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		bidi := client.MergeProfilesStacktraces(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))
		cancel()
	})

	t.Run("test close request", func(t *testing.T) {
		bidi := client.MergeProfilesStacktraces(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))
		require.NoError(t, bidi.CloseRequest())
	})
}

func TestMergeProfilesPprof(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// ingest some sample data
	var (
		ctx     = testContext(t)
		testDir = contextDataDir(ctx)
		end     = time.Unix(0, int64(time.Hour))
		start   = end.Add(-time.Minute)
		step    = 15 * time.Second
	)

	db, err := New(ctx, Config{
		DataPath:         testDir,
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	client, cleanup := db.queriers().ingesterClient()
	defer cleanup()

	t.Run("request the one existing series", func(t *testing.T) {
		bidi := client.MergeProfilesPprof(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))

		resp, err := bidi.Receive()
		require.NoError(t, err)
		require.Nil(t, resp.Result)
		require.Len(t, resp.SelectedProfiles.LabelsSets, 1)
		require.Len(t, resp.SelectedProfiles.Profiles, 5)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Profiles: []bool{true},
		}))

		// expect empty resp to signal it is finished
		resp, err = bidi.Receive()
		require.NoError(t, err)
		require.Nil(t, resp.Result)

		// received result
		resp, err = bidi.Receive()
		require.NoError(t, err)
		require.NotNil(t, resp.Result)
		p, err := profile.ParseUncompressed(resp.Result)
		require.NoError(t, err)
		require.Len(t, p.Sample, 48)
		require.Len(t, p.Location, 287)
	})

	t.Run("request non existing series", func(t *testing.T) {
		bidi := client.MergeProfilesPprof(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="not-my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))

		// expect empty resp to signal it is finished
		resp, err := bidi.Receive()
		require.NoError(t, err)
		require.Nil(t, resp.Result)
		require.Nil(t, resp.SelectedProfiles)

		// still receiving a result
		resp, err = bidi.Receive()
		require.NoError(t, err)
		require.NotNil(t, resp.Result)
		p, err := profile.ParseUncompressed(resp.Result)
		require.NoError(t, err)
		require.Len(t, p.Sample, 0)
		require.Len(t, p.Location, 0)
		require.Nil(t, resp.SelectedProfiles)
	})

	t.Run("empty request fails", func(t *testing.T) {
		bidi := client.MergeProfilesPprof(ctx)

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{}))

		_, err := bidi.Receive()
		require.EqualError(t, err, "invalid_argument: missing initial select request")
	})

	t.Run("test cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		bidi := client.MergeProfilesPprof(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))
		cancel()
	})

	t.Run("test close request", func(t *testing.T) {
		bidi := client.MergeProfilesPprof(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         start.UnixMilli(),
				End:           end.UnixMilli(),
			},
		}))
		require.NoError(t, bidi.CloseRequest())
	})

	t.Run("timerange with no Profiles", func(t *testing.T) {
		bidi := client.MergeProfilesPprof(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{
				LabelSelector: `{pod="my-pod"}`,
				Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
				Start:         0,
				End:           1,
			},
		}))
		_, err := bidi.Receive()
		require.NoError(t, err)
		_, err = bidi.Receive()
		require.NoError(t, err)
	})
}

func Test_QueryNotInitializedHead(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := testContext(t)

	db, err := New(ctx, Config{
		DataPath:         contextDataDir(ctx),
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	client, cleanup := db.queriers().ingesterClient()
	defer cleanup()

	t.Run("ProfileTypes", func(t *testing.T) {
		resp, err := db.ProfileTypes(ctx, connect.NewRequest(new(ingestv1.ProfileTypesRequest)))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Msg)
	})

	t.Run("LabelNames", func(t *testing.T) {
		resp, err := db.LabelNames(ctx, connect.NewRequest(new(typesv1.LabelNamesRequest)))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Msg)
	})

	t.Run("LabelValues", func(t *testing.T) {
		resp, err := db.LabelValues(ctx, connect.NewRequest(new(typesv1.LabelValuesRequest)))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Msg)
	})

	t.Run("Series", func(t *testing.T) {
		resp, err := db.Series(ctx, connect.NewRequest(&ingestv1.SeriesRequest{}))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Msg)
	})

	t.Run("MergeProfilesLabels", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		bidi := client.MergeProfilesLabels(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesLabelsRequest{
			Request: &ingestv1.SelectProfilesRequest{},
		}))
		cancel()
	})

	t.Run("MergeProfilesStacktraces", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		bidi := client.MergeProfilesStacktraces(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
			Request: &ingestv1.SelectProfilesRequest{},
		}))
		cancel()
	})

	t.Run("MergeProfilesPprof", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		bidi := client.MergeProfilesPprof(ctx)
		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{
			Request: &ingestv1.SelectProfilesRequest{},
		}))
		cancel()
	})
}

func Test_FlushNotInitializedHead(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := testContext(t)

	db, err := New(ctx, Config{
		DataPath:         contextDataDir(ctx),
		MaxBlockDuration: 1 * time.Hour,
	}, NoLimit, ctx.localBucketClient)

	var (
		end   = time.Unix(0, int64(time.Hour))
		start = end.Add(-time.Minute)
		step  = 5 * time.Second
	)

	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)
	require.NoError(t, db.Flush(ctx, true, ""))
	require.Zero(t, db.headSize())

	require.NoError(t, db.Flush(ctx, true, ""))
	require.Zero(t, db.headSize())

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	require.NotZero(t, db.headSize())
	require.NoError(t, db.Flush(ctx, true, ""))
}

func Test_endRangeForTimestamp(t *testing.T) {
	for _, tt := range []struct {
		name     string
		ts       int64
		expected int64
	}{
		{
			name:     "start of first range",
			ts:       0,
			expected: 1 * time.Hour.Nanoseconds(),
		},
		{
			name:     "end of first range",
			ts:       1*time.Hour.Nanoseconds() - 1,
			expected: 1 * time.Hour.Nanoseconds(),
		},
		{
			name:     "start of second range",
			ts:       1 * time.Hour.Nanoseconds(),
			expected: 2 * time.Hour.Nanoseconds(),
		},
		{
			name:     "end of second range",
			ts:       2*time.Hour.Nanoseconds() - 1,
			expected: 2 * time.Hour.Nanoseconds(),
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, endRangeForTimestamp(tt.ts, 1*time.Hour.Nanoseconds()))
		})
	}
}

func Test_getProfileStatsFromMetas(t *testing.T) {
	tests := []struct {
		name     string
		minTimes []model.Time
		maxTimes []model.Time
		want     *typesv1.GetProfileStatsResponse
	}{
		{
			name:     "no metas should result in no data ingested",
			minTimes: []model.Time{},
			maxTimes: []model.Time{},
			want: &typesv1.GetProfileStatsResponse{
				DataIngested:      false,
				OldestProfileTime: math.MaxInt64,
				NewestProfileTime: math.MinInt64,
			},
		},
		{
			name: "valid metas should result in data ingested",
			minTimes: []model.Time{
				model.TimeFromUnix(1710161819),
				model.TimeFromUnix(1710171819),
			},
			maxTimes: []model.Time{
				model.TimeFromUnix(1710172239),
				model.TimeFromUnix(1710174239),
			},
			want: &typesv1.GetProfileStatsResponse{
				DataIngested:      true,
				OldestProfileTime: 1710161819000,
				NewestProfileTime: 1710174239000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := getProfileStatsFromBounds(tt.minTimes, tt.maxTimes)
			require.NoError(t, err)
			assert.Equalf(t, tt.want, response, "getProfileStatsFromBounds(%v, %v)", tt.minTimes, tt.maxTimes)
		})
	}
}
