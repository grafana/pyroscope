package phlaredb

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/mock"
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
	diskutil "github.com/grafana/phlare/pkg/util/disk"
)

func TestCreateLocalDir(t *testing.T) {
	dataPath := t.TempDir()
	localFile := dataPath + "/local"
	require.NoError(t, ioutil.WriteFile(localFile, []byte("d"), 0o644))
	_, err := New(context.Background(), Config{
		DataPath:         dataPath,
		MaxBlockDuration: 30 * time.Minute,
	})
	require.Error(t, err)
	require.NoError(t, os.Remove(localFile))
	_, err = New(context.Background(), Config{
		DataPath:         dataPath,
		MaxBlockDuration: 30 * time.Minute,
	})
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
		require.NoError(b, db.Head().Ingest(
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

func (i *ingesterHandlerPhlareDB) Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) LabelValues(context.Context, *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *ingesterHandlerPhlareDB) LabelNames(context.Context, *connect.Request[ingestv1.LabelNamesRequest]) (*connect.Response[ingestv1.LabelNamesResponse], error) {
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
	})
	require.NoError(t, err)
	defer require.NoError(t, db.Close())

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	// create client
	ctx := context.Background()

	client, cleanup := db.Queriers().ingesterClient()
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
		require.Len(t, resp.Result.Stacktraces, 48)
		require.Len(t, resp.Result.FunctionNames, 247)
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
	})
	require.NoError(t, err)
	defer require.NoError(t, db.Close())

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	// create client
	ctx := context.Background()

	client, cleanup := db.Queriers().ingesterClient()
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
}

func TestFilterProfiles(t *testing.T) {
	ctx := context.Background()
	profiles := lo.Times(11, func(i int) Profile {
		return ProfileWithLabels{
			Profile: &schemav1.Profile{TimeNanos: int64(i * int(time.Minute))},
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
		*ingestv1.MergeProfilesStacktracesRequest](ctx, in, 5, bidi)
	require.NoError(t, err)
	require.Equal(t, 2, len(filtered))
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
			Profile: &schemav1.Profile{TimeNanos: int64(5 * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)).Hash()),
		},
		ProfileWithLabels{
			Profile: &schemav1.Profile{TimeNanos: int64(10 * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)).Hash()),
		},
	}, filtered)
}

type fakeBlock struct {
	id   string
	size uint64 // in mbytes
}

type fakeVolumeFS struct {
	mock.Mock
	blocks []fakeBlock
}

func (f *fakeVolumeFS) HasHighDiskUtilization(path string) (*diskutil.VolumeStats, error) {
	args := f.Called(path)
	return args[0].(*diskutil.VolumeStats), args.Error(1)
}

func (f *fakeVolumeFS) Open(path string) (fs.File, error) {
	args := f.Called(path)
	return args[0].(fs.File), args.Error(1)
}
func (f *fakeVolumeFS) RemoveAll(path string) error {
	args := f.Called(path)
	return args.Error(0)
}

func (f *fakeVolumeFS) ReadDir(path string) ([]fs.DirEntry, error) {
	args := f.Called(path)
	return args[0].([]fs.DirEntry), args.Error(1)
}

type fakeFile struct {
	name string
	dir  bool
}

func (f *fakeFile) Name() string               { return f.name }
func (f *fakeFile) IsDir() bool                { return f.dir }
func (f *fakeFile) Info() (fs.FileInfo, error) { panic("not implemented") }
func (f *fakeFile) Type() fs.FileMode          { panic("not implemented") }

func TestPhlareDB_cleanupBlocksWhenHighDiskUtilization(t *testing.T) {
	const suffix = "0000000000000000000000"

	for _, tc := range []struct {
		name     string
		mock     func(fs *fakeVolumeFS)
		logLines []string
		err      string
	}{
		{
			name: "no-high-disk-utilization",
			mock: func(f *fakeVolumeFS) {
				f.On("HasHighDiskUtilization", mock.Anything).Return(&diskutil.VolumeStats{HighDiskUtilization: false}, nil).Once()
			},
		},
		{
			name: "high-disk-utilization-no-blocks",
			mock: func(f *fakeVolumeFS) {
				f.On("HasHighDiskUtilization", mock.Anything).Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", mock.Anything).Return([]fs.DirEntry{&fakeFile{"just-a-file", false}}, nil).Once()
			},
		},
		{
			name: "high-disk-utilization-delete-single-block",
			mock: func(f *fakeVolumeFS) {
				f.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				f.On("RemoveAll", "local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: false, BytesAvailable: 11}, nil).Once()
			},
			logLines: []string{`{"level":"warn", "msg":"disk utilization is high, deleted oldest block", "path":"local/01AA0000000000000000000000"}`},
		},
		{
			name: "high-disk-utilization-delete-two-blocks",
			mock: func(f *fakeVolumeFS) {
				f.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				f.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				f.On("RemoveAll", "local/01AA"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 11}, nil).Once()
				f.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
				}, nil).Once()
				f.On("RemoveAll", "local/01AB"+suffix).Return(nil).Once()
				f.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: false, BytesAvailable: 12}, nil).Once()
			},
			logLines: []string{
				`{"level":"warn", "msg":"disk utilization is high, deleted oldest block", "path":"local/01AA0000000000000000000000"}`,
				`{"level":"warn", "msg":"disk utilization is high, deleted oldest block", "path":"local/01AB0000000000000000000000"}`,
			},
		},
		{
			name: "high-disk-utilization-delete-blocks-no-reduction-in-usage",
			mock: func(fakeFS *fakeVolumeFS) {
				fakeFS.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				fakeFS.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				fakeFS.On("RemoveAll", "local/01AA"+suffix).Return(nil).Once()
				fakeFS.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				fakeFS.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
				}, nil).Once()
				fakeFS.On("RemoveAll", "local/01AB"+suffix).Return(nil).Once()
				fakeFS.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
			},
			logLines: []string{
				`{"level":"warn", "msg":"disk utilization is high, deleted oldest block", "path":"local/01AA0000000000000000000000"}`,
				`{"level":"warn", "msg":"disk utilization is not lowered by deletion of block, pausing until next cycle", "path":"local"}`,
			},
		},
		{
			name: "high-disk-utilization-delete-blocks-block-not-removed",
			mock: func(fakeFS *fakeVolumeFS) {
				fakeFS.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 10}, nil).Once()
				fakeFS.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
				fakeFS.On("RemoveAll", "local/01AA"+suffix).Return(nil).Once()
				fakeFS.On("HasHighDiskUtilization", "local").Return(&diskutil.VolumeStats{HighDiskUtilization: true, BytesAvailable: 11}, nil).Once()
				fakeFS.On("ReadDir", mock.Anything).Return([]fs.DirEntry{
					&fakeFile{"01AC" + suffix, true},
					&fakeFile{"01AB" + suffix, true},
					&fakeFile{"01AA" + suffix, true},
				}, nil).Once()
			},
			err: "making no progress in deletion: trying to delete block '01AA0000000000000000000000' again",
			logLines: []string{
				`{"level":"warn", "msg":"disk utilization is high, deleted oldest block", "path":"local/01AA0000000000000000000000"}`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				logBuf = bytes.NewBuffer(nil)
				logger = log.NewJSONLogger(log.NewSyncWriter(logBuf))
				ctx    = context.Background()
				fakeFS = &fakeVolumeFS{}
			)

			db := &PhlareDB{
				logger:        logger,
				volumeChecker: fakeFS,
				fs:            fakeFS,
			}

			tc.mock(fakeFS)

			if tc.err == "" {
				require.NoError(t, db.cleanupBlocksWhenHighDiskUtilization(ctx))
			} else {
				require.Equal(t, tc.err, db.cleanupBlocksWhenHighDiskUtilization(ctx).Error())
			}

			// check for log lines
			if len(tc.logLines) > 0 {
				lines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
				require.Len(t, lines, len(tc.logLines))
				for idx := range tc.logLines {
					require.JSONEq(t, tc.logLines[idx], lines[idx])
				}
			}
		})
	}
}
