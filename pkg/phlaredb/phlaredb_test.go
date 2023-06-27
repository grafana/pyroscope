package phlaredb

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/api/gen/proto/go/ingester/v1/ingesterv1connect"
	pushv1 "github.com/grafana/phlare/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/testhelper"
)

func TestCreateLocalDir(t *testing.T) {
	dataPath := t.TempDir()
	localFile := dataPath + "/local"
	require.NoError(t, os.WriteFile(localFile, []byte("d"), 0o644))
	_, err := New(context.Background(), Config{
		DataPath:         dataPath,
		MaxBlockDuration: 30 * time.Minute,
	}, NoLimit)
	require.Error(t, err)
	require.NoError(t, os.Remove(localFile))
	_, err = New(context.Background(), Config{
		DataPath:         dataPath,
		MaxBlockDuration: 30 * time.Minute,
	}, NoLimit)
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
	return MergeProfilesStacktraces(ctx, stream, i.ForTimeRange)
}

func (i *ingesterHandlerPhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return MergeProfilesLabels(ctx, stream, i.ForTimeRange)
}

func (i *ingesterHandlerPhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return MergeProfilesPprof(ctx, stream, i.ForTimeRange)
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

func TestMergeProfilesStacktraces(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// ingest some sample data
	var (
		testDir = t.TempDir()
		end     = time.Unix(0, int64(time.Hour))
		start   = end.Add(-time.Minute)
		step    = 15 * time.Second
	)

	db, err := New(context.Background(), Config{
		DataPath:         testDir,
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	// create client
	ctx := context.Background()

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
		testDir = t.TempDir()
		end     = time.Unix(0, int64(time.Hour))
		start   = end.Add(-time.Minute)
		step    = 15 * time.Second
	)

	db, err := New(context.Background(), Config{
		DataPath:         testDir,
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	// create client
	ctx := context.Background()

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

func TestFilterProfiles(t *testing.T) {
	ctx := context.Background()
	profiles := lo.Times(11, func(i int) Profile {
		return ProfileWithLabels{
			profile: &schemav1.InMemoryProfile{TimeNanos: int64(i * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)).Hash()),
		}
	})
	in := iter.NewSliceIterator(profiles)
	bidi := &fakeBidiServerMergeProfilesStacktraces{
		keep: [][]bool{{}, {true}, {true}},
		t:    t,
	}
	filtered, err := filterProfiles[
		BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest],
		*ingestv1.MergeProfilesStacktracesResponse,
		*ingestv1.MergeProfilesStacktracesRequest](ctx, []iter.Iterator[Profile]{in}, 5, bidi)
	require.NoError(t, err)
	require.Equal(t, 2, len(filtered[0]))
	require.Equal(t, 3, len(bidi.profilesSent))
	testhelper.EqualProto(t, []*ingestv1.ProfileSets{
		{
			LabelsSets: lo.Times(5, func(i int) *typesv1.Labels {
				return &typesv1.Labels{Labels: phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i))}
			}),
			Profiles: lo.Times(5, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64(i * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
		{
			LabelsSets: lo.Times(5, func(i int) *typesv1.Labels {
				return &typesv1.Labels{Labels: phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i+5))}
			}),
			Profiles: lo.Times(5, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64((i + 5) * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
		{
			LabelsSets: lo.Times(1, func(i int) *typesv1.Labels {
				return &typesv1.Labels{Labels: phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i+10))}
			}),
			Profiles: lo.Times(1, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64((i + 10) * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
	}, bidi.profilesSent)

	require.Equal(t, []Profile{
		ProfileWithLabels{
			profile: &schemav1.InMemoryProfile{TimeNanos: int64(5 * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)).Hash()),
		},
		ProfileWithLabels{
			profile: &schemav1.InMemoryProfile{TimeNanos: int64(10 * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)).Hash()),
		},
	}, filtered[0])
}

func Test_QueryNotInitializedHead(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	db, err := New(context.Background(), Config{
		DataPath:         t.TempDir(),
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	ctx := context.Background()
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

	db, err := New(context.Background(), Config{
		DataPath: t.TempDir(),
	}, NoLimit)

	var (
		end   = time.Unix(0, int64(time.Hour))
		start = end.Add(-time.Minute)
		step  = 5 * time.Second
	)

	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	require.Equal(t, db.headFlushCh(), db.stopCh)
	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	ctx := context.Background()
	c1 := db.headFlushCh()
	require.NotEqual(t, db.stopCh, c1)
	require.NoError(t, db.Flush(ctx))

	// After flush and before the next head is initialized, stopCh is expected.
	c2 := db.headFlushCh()
	require.Equal(t, db.stopCh, c2)

	// Once data is getting in, the head flush channel is updated.
	require.Equal(t, db.headFlushCh(), db.stopCh)
	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)
	c3 := db.headFlushCh()
	require.NotEqual(t, c2, c3)
	require.NotEqual(t, db.stopCh, c3)
	require.NoError(t, db.Flush(ctx))

	require.Equal(t, db.stopCh, db.headFlushCh())
	require.NoError(t, db.Flush(ctx)) // Nil head flush does not cause errors.
}
