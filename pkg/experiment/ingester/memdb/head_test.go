package memdb

import (
	"bytes"
	"connectrpc.com/connect"
	"context"
	"fmt"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/ingester/memdb/testutil"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	testutil2 "github.com/grafana/pyroscope/pkg/phlaredb/block/testutil"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestHeadLabelValues(t *testing.T) {
	head := newTestHead()
	head.Ingest(newProfileFoo(), uuid.New(), []*typesv1.LabelPair{{Name: "job", Value: "foo"}, {Name: "namespace", Value: "phlare"}})
	head.Ingest(newProfileBar(), uuid.New(), []*typesv1.LabelPair{{Name: "job", Value: "bar"}, {Name: "namespace", Value: "phlare"}})

	q := flushTestHead(t, head)

	res, err := q.LabelValues(context.Background(), connect.NewRequest(&typesv1.LabelValuesRequest{Name: "cluster"}))
	require.NoError(t, err)
	require.Equal(t, []string{}, res.Msg.Names)

	res, err = q.LabelValues(context.Background(), connect.NewRequest(&typesv1.LabelValuesRequest{Name: "job"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, res.Msg.Names)
}
func TestHeadLabelNames(t *testing.T) {
	head := newTestHead()
	head.Ingest(newProfileFoo(), uuid.New(), []*typesv1.LabelPair{{Name: "job", Value: "foo"}, {Name: "namespace", Value: "phlare"}})
	head.Ingest(newProfileBar(), uuid.New(), []*typesv1.LabelPair{{Name: "job", Value: "bar"}, {Name: "namespace", Value: "phlare"}})

	q := flushTestHead(t, head)

	res, err := q.LabelNames(context.Background(), connect.NewRequest(&typesv1.LabelNamesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"__period_type__", "__period_unit__", "__profile_type__", "__type__", "__unit__", "job", "namespace"}, res.Msg.Names)
}

func TestHeadSeries(t *testing.T) {
	head := newTestHead()
	fooLabels := phlaremodel.NewLabelsBuilder(nil).Set("namespace", "phlare").Set("job", "foo").Labels()
	barLabels := phlaremodel.NewLabelsBuilder(nil).Set("namespace", "phlare").Set("job", "bar").Labels()
	head.Ingest(newProfileFoo(), uuid.New(), fooLabels)
	head.Ingest(newProfileBar(), uuid.New(), barLabels)

	lblBuilder := phlaremodel.NewLabelsBuilder(nil).
		Set("namespace", "phlare").
		Set("job", "foo").
		Set("__period_type__", "type").
		Set("__period_unit__", "unit").
		Set("__type__", "type").
		Set("__unit__", "unit").
		Set("__profile_type__", ":type:unit:type:unit")
	expected := lblBuilder.Labels()

	q := flushTestHead(t, head)

	res, err := q.Series(context.Background(), &ingestv1.SeriesRequest{Matchers: []string{`{job="foo"}`}})
	require.NoError(t, err)
	require.Equal(t, []*typesv1.Labels{{Labels: expected}}, res)

	// Test we can filter labelNames
	res, err = q.Series(context.Background(), &ingestv1.SeriesRequest{LabelNames: []string{"job", "not-existing"}})
	require.NoError(t, err)
	lblBuilder.Reset(nil)
	jobFoo := lblBuilder.Set("job", "foo").Labels()
	lblBuilder.Reset(nil)
	jobBar := lblBuilder.Set("job", "bar").Labels()
	require.Len(t, res, 2)
	require.Contains(t, res, &typesv1.Labels{Labels: jobFoo})
	require.Contains(t, res, &typesv1.Labels{Labels: jobBar})
}

func TestHeadProfileTypes(t *testing.T) {
	head := newTestHead()
	head.Ingest(newProfileFoo(), uuid.New(), []*typesv1.LabelPair{{Name: "__name__", Value: "foo"}, {Name: "job", Value: "foo"}, {Name: "namespace", Value: "phlare"}})
	head.Ingest(newProfileBar(), uuid.New(), []*typesv1.LabelPair{{Name: "__name__", Value: "bar"}, {Name: "namespace", Value: "phlare"}})

	q := flushTestHead(t, head)

	res, err := q.ProfileTypes(context.Background(), connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []*typesv1.ProfileType{
		mustParseProfileSelector(t, "bar:type:unit:type:unit"),
		mustParseProfileSelector(t, "foo:type:unit:type:unit"),
	}, res.Msg.ProfileTypes)
}

func TestHead_SelectMatchingProfiles_Order(t *testing.T) {
	const n = 15
	head := NewHead(NewHeadMetricsWithPrefix(nil, ""))

	now := time.Now()
	for i := 0; i < n; i++ {
		x := newProfileFoo()
		// Make sure some of our profiles have matching timestamps.
		x.TimeNanos = now.Add(time.Second * time.Duration(i-i%2)).UnixNano()
		head.Ingest(x, uuid.UUID{}, []*typesv1.LabelPair{
			{Name: "job", Value: "foo"},
			{Name: "x", Value: strconv.Itoa(i)},
		})
	}

	q := flushTestHead(t, head)

	typ, err := phlaremodel.ParseProfileTypeSelector(":type:unit:type:unit")
	require.NoError(t, err)
	req := &ingestv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          typ,
		End:           now.Add(time.Hour).UnixMilli(),
	}

	profiles := make([]phlaredb.Profile, 0, n)
	i, err := q.SelectMatchingProfiles(context.Background(), req)
	require.NoError(t, err)
	s, err := iter.Slice(i)
	require.NoError(t, err)
	profiles = append(profiles, s...)

	assert.Equal(t, n, len(profiles))
	for i, p := range profiles {
		x, err := strconv.Atoi(p.Labels().Get("x"))
		require.NoError(t, err)
		require.Equal(t, i, x, "SelectMatchingProfiles order mismatch")
	}
}

const testdataPrefix = "../../../phlaredb"

func TestHeadFlushQuery(t *testing.T) {
	testdata := []struct {
		path    string
		profile *profilev1.Profile
		svc     string
	}{
		{testdataPrefix + "/testdata/heap", nil, "svc_heap"},
		{testdataPrefix + "/testdata/profile", nil, "svc_profile"},
		{testdataPrefix + "/testdata/profile_uncompressed", nil, "svc_profile_uncompressed"},
		{testdataPrefix + "/testdata/profile_python", nil, "svc_python"},
		{testdataPrefix + "/testdata/profile_java", nil, "svc_java"},
	}
	for i := range testdata {
		td := &testdata[i]
		p := parseProfile(t, td.path)
		td.profile = p
	}

	head := newTestHead()
	ctx := context.Background()

	for pos := range testdata {
		head.Ingest(testdata[pos].profile.CloneVT(), uuid.New(), []*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameServiceName, Value: testdata[pos].svc},
		})
	}

	flushed, err := head.Flush(ctx)
	require.NoError(t, err)

	assert.Equal(t, 14192, int(flushed.Meta.NumSamples))
	assert.Equal(t, 11, int(flushed.Meta.NumSeries)) // different value from original phlaredb test because service_name label added
	assert.Equal(t, 11, int(flushed.Meta.NumProfiles))
	assert.Equal(t, []string{
		":CPU:nanoseconds:CPU:nanoseconds",
		":alloc_objects:count:space:bytes",
		":alloc_space:bytes:space:bytes",
		":cpu:nanoseconds:cpu:nanoseconds",
		":inuse_objects:count:space:bytes",
		":inuse_space:bytes:space:bytes",
		":sample:count:CPU:nanoseconds",
		":samples:count:cpu:nanoseconds",
	}, flushed.Meta.ProfileTypeNames)

	q := createBlockFromFlushedHead(t, flushed)

	for _, td := range testdata {
		for stIndex := range td.profile.SampleType {
			p, err := q.SelectMergePprof(context.Background(), &ingestv1.SelectProfilesRequest{
				LabelSelector: fmt.Sprintf("{%s=\"%s\"}", phlaremodel.LabelNameServiceName, td.svc),
				Type:          profileTypeFromProfile(td.profile, stIndex),
				Start:         time.Unix(0, td.profile.TimeNanos).UnixMilli(),
				End:           time.Unix(0, td.profile.TimeNanos).Add(time.Millisecond).UnixMilli(),
			}, 163840, nil,
			)
			require.NoError(t, err)
			require.NotNil(t, p)

			compareProfile(t, td.profile, stIndex, p)
		}
	}
}

func TestHead_Concurrent_Ingest(t *testing.T) {
	head := newTestHead()

	wg := sync.WaitGroup{}

	profilesPerSeries := 330

	for i := 0; i < 3; i++ {
		wg.Add(1)
		// ingester
		go func(i int) {
			defer wg.Done()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for j := 0; j < profilesPerSeries; j++ {
				<-tick.C
				ingestThreeProfileStreams(profilesPerSeries*i+j, head.Ingest)
			}
			t.Logf("ingest stream %s done", streams[i])
		}(i)
	}

	wg.Wait()

	_ = flushTestHead(t, head)
}

func profileWithID(id int) (*profilev1.Profile, uuid.UUID) {
	p := newProfileFoo()
	p.TimeNanos = int64(id)
	return p, uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", id))
}

func TestHead_ProfileOrder(t *testing.T) {
	head := newTestHead()

	p, u := profileWithID(1)
	head.Ingest(
		p,
		u,
		[]*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileName, Value: "memory"},
			{Name: phlaremodel.LabelNameOrder, Value: phlaremodel.LabelOrderEnforced},
			{Name: phlaremodel.LabelNameServiceName, Value: "service-a"},
		},
	)

	p, u = profileWithID(2)
	head.Ingest(
		p,
		u,
		[]*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileName, Value: "memory"},
			{Name: phlaremodel.LabelNameOrder, Value: phlaremodel.LabelOrderEnforced},
			{Name: phlaremodel.LabelNameServiceName, Value: "service-b"},
			{Name: "____Label", Value: "important"},
		},
	)

	p, u = profileWithID(3)
	head.Ingest(
		p,
		u,
		[]*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileName, Value: "memory"},
			{Name: phlaremodel.LabelNameOrder, Value: phlaremodel.LabelOrderEnforced},
			{Name: phlaremodel.LabelNameServiceName, Value: "service-c"},
			{Name: "AAALabel", Value: "important"},
		},
	)

	p, u = profileWithID(4)
	head.Ingest(
		p,
		u,
		[]*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileName, Value: "cpu"},
			{Name: phlaremodel.LabelNameOrder, Value: phlaremodel.LabelOrderEnforced},
			{Name: phlaremodel.LabelNameServiceName, Value: "service-a"},
			{Name: "000Label", Value: "important"},
		},
	)

	p, u = profileWithID(5)
	head.Ingest(
		p,
		u,
		[]*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileName, Value: "cpu"},
			{Name: phlaremodel.LabelNameOrder, Value: phlaremodel.LabelOrderEnforced},
			{Name: phlaremodel.LabelNameServiceName, Value: "service-b"},
		},
	)

	p, u = profileWithID(6)
	head.Ingest(
		p,
		u,
		[]*typesv1.LabelPair{
			{Name: phlaremodel.LabelNameProfileName, Value: "cpu"},
			{Name: phlaremodel.LabelNameOrder, Value: phlaremodel.LabelOrderEnforced},
			{Name: phlaremodel.LabelNameServiceName, Value: "service-b"},
		},
	)

	flushed, err := head.Flush(context.Background())
	require.NoError(t, err)

	// test that the profiles are ordered correctly
	type row struct{ TimeNanos uint64 }
	rows, err := parquet.Read[row](bytes.NewReader(flushed.Profiles), int64(len(flushed.Profiles)))
	require.NoError(t, err)
	require.Equal(t, []row{
		{4}, {5}, {6}, {1}, {2}, {3},
	}, rows)
}

func TestFlushEmptyHead(t *testing.T) {
	head := newTestHead()
	flushed, err := head.Flush(context.Background())
	require.NoError(t, err)
	require.NotNil(t, flushed)
	require.Equal(t, 0, len(flushed.Profiles))
}

func TestMergeProfilesStacktraces(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// ingest some sample data
	var (
		end   = time.Unix(0, int64(time.Hour))
		start = end.Add(-time.Minute)
		step  = 15 * time.Second
	)

	db := newTestHead()

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	q := flushTestHead(t, db)

	// create client
	client, cleanup := testutil.IngesterClientForTest(t, []phlaredb.Querier{q})
	defer cleanup()

	t.Run("request the one existing series", func(t *testing.T) {
		bidi := client.MergeProfilesStacktraces(context.Background())

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
		require.Len(t, resp.SelectedProfiles.Fingerprints, 1)
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
		bidi := client.MergeProfilesStacktraces(context.Background())

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
		bidi := client.MergeProfilesStacktraces(context.Background())

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{}))

		_, err := bidi.Receive()
		require.EqualError(t, err, "invalid_argument: missing initial select request")
	})

	t.Run("test cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
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
		bidi := client.MergeProfilesStacktraces(context.Background())
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
		end   = time.Unix(0, int64(time.Hour))
		start = end.Add(-time.Minute)
		step  = 15 * time.Second
	)

	db := NewHead(NewHeadMetricsWithPrefix(nil, ""))

	ingestProfiles(t, db, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
	)

	q := flushTestHead(t, db)

	// create client
	client, cleanup := testutil.IngesterClientForTest(t, []phlaredb.Querier{q})
	defer cleanup()

	t.Run("request the one existing series", func(t *testing.T) {
		bidi := client.MergeProfilesPprof(context.Background())

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
		require.Len(t, resp.SelectedProfiles.Fingerprints, 1)
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
		bidi := client.MergeProfilesPprof(context.Background())

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
		bidi := client.MergeProfilesPprof(context.Background())

		require.NoError(t, bidi.Send(&ingestv1.MergeProfilesPprofRequest{}))

		_, err := bidi.Receive()
		require.EqualError(t, err, "invalid_argument: missing initial select request")
	})

	t.Run("test cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
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
		bidi := client.MergeProfilesPprof(context.Background())
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
		bidi := client.MergeProfilesPprof(context.Background())
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

// See https://github.com/grafana/pyroscope/pull/3356
func Test_HeadFlush_DuplicateLabels(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// ingest some sample data
	var (
		end   = time.Unix(0, int64(time.Hour))
		start = end.Add(-time.Minute)
		step  = 15 * time.Second
	)

	head := newTestHead()

	ingestProfiles(t, head, cpuProfileGenerator, start.UnixNano(), end.UnixNano(), step,
		&typesv1.LabelPair{Name: "namespace", Value: "my-namespace"},
		&typesv1.LabelPair{Name: "pod", Value: "my-pod"},
		&typesv1.LabelPair{Name: "pod", Value: "not-my-pod"},
	)
}
func BenchmarkHeadIngestProfiles(t *testing.B) {
	var (
		profilePaths = []string{
			testdataPrefix + "/testdata/heap",
			testdataPrefix + "/testdata/profile",
		}
		profileCount = 0
	)

	head := newTestHead()

	t.ReportAllocs()

	for n := 0; n < t.N; n++ {
		for pos := range profilePaths {
			p := parseProfile(t, profilePaths[pos])
			head.Ingest(p, uuid.New(), []*typesv1.LabelPair{})
			profileCount++
		}
	}
}

func newTestHead() *Head {
	head := NewHead(NewHeadMetricsWithPrefix(nil, ""))
	return head
}

func parseProfile(t testing.TB, path string) *profilev1.Profile {
	p, err := pprof.OpenFile(path)
	require.NoError(t, err, "failed opening profile: ", path)
	if p.Profile.Mapping == nil {
		// Add fake mappings to some profiles, otherwise query may panic in symdb or return wrong unpredictable results
		p.Mapping = []*profilev1.Mapping{
			{Id: 0},
		}
	}
	return p.Profile
}

var valueTypeStrings = []string{"unit", "type"}

func newValueType() *profilev1.ValueType {
	return &profilev1.ValueType{
		Unit: 1,
		Type: 2,
	}
}

func newProfileFoo() *profilev1.Profile {
	baseTable := append([]string{""}, valueTypeStrings...)
	baseTableLen := int64(len(baseTable)) + 0
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: baseTableLen + 0,
			},
			{
				Id:   2,
				Name: baseTableLen + 1,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   0x1337,
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   0x1338,
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: baseTableLen + 2},
		},
		StringTable: append(baseTable, []string{
			"func_a",
			"func_b",
			"my-foo-binary",
		}...),
		TimeNanos:  123456,
		PeriodType: newValueType(),
		SampleType: []*profilev1.ValueType{newValueType()},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{0o123},
				LocationId: []uint64{1},
			},
			{
				Value:      []int64{1234},
				LocationId: []uint64{1, 2},
			},
		},
	}
}

func newProfileBar() *profilev1.Profile {
	baseTable := append([]string{""}, valueTypeStrings...)
	baseTableLen := int64(len(baseTable)) + 0
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   10,
				Name: baseTableLen + 1,
			},
			{
				Id:   21,
				Name: baseTableLen + 0,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        113,
				MappingId: 1,
				Address:   0x1337,
				Line: []*profilev1.Line{
					{FunctionId: 10, Line: 1},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: baseTableLen + 2},
		},
		StringTable: append(baseTable, []string{
			"func_b",
			"func_a",
			"my-bar-binary",
		}...),
		TimeNanos:  123456,
		PeriodType: newValueType(),
		SampleType: []*profilev1.ValueType{newValueType()},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{2345},
				LocationId: []uint64{113},
			},
		},
	}
}

var streams = []string{"stream-a", "stream-b", "stream-c"}

func ingestThreeProfileStreams(i int, ingest func(*profilev1.Profile, uuid.UUID, []*typesv1.LabelPair)) {
	p := testhelper.NewProfileBuilder(time.Second.Nanoseconds() * int64(i))
	p.CPUProfile()
	p.WithLabels(
		"job", "foo",
		"stream", streams[i%3],
	)
	p.UUID = uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	p.ForStacktraceString("func1", "func2").AddSamples(10)
	p.ForStacktraceString("func1").AddSamples(20)

	ingest(p.Profile, p.UUID, p.Labels)
}

func profileTypeFromProfile(p *profilev1.Profile, stIndex int) *typesv1.ProfileType {
	t := &typesv1.ProfileType{
		SampleType: p.StringTable[p.SampleType[stIndex].Type],
		SampleUnit: p.StringTable[p.SampleType[stIndex].Unit],
		PeriodType: p.StringTable[p.PeriodType.Type],
		PeriodUnit: p.StringTable[p.PeriodType.Unit],
	}
	t.ID = fmt.Sprintf(":%s:%s:%s:%s", t.SampleType, t.SampleUnit, t.PeriodType, t.PeriodUnit)
	return t
}

func compareProfile(t *testing.T, expected *profilev1.Profile, expectedSampleTypeIndex int, actual *profilev1.Profile) {
	actualCollapsed := bench.StackCollapseProto(actual, 0, 1.0)
	expectedCollapsed := bench.StackCollapseProto(expected, expectedSampleTypeIndex, 1.0)
	assert.Equal(t, expectedCollapsed, actualCollapsed)
}

func flushTestHead(t *testing.T, head *Head) phlaredb.Querier {
	flushed, err := head.Flush(context.Background())
	require.NoError(t, err)

	q := createBlockFromFlushedHead(t, flushed)
	return q
}

func createBlockFromFlushedHead(t *testing.T, flushed *FlushedHead) phlaredb.Querier {
	dir := t.TempDir()
	block := testutil2.OpenBlockFromMemory(t, dir, model.TimeFromUnixNano(flushed.Meta.MinTimeNanos), model.TimeFromUnixNano(flushed.Meta.MinTimeNanos), flushed.Profiles, flushed.Index, flushed.Symbols)
	q := block.Queriers()
	err := q.Open(context.Background())
	require.NoError(t, err)
	require.Len(t, q, 1)
	return q[0]
}

func mustParseProfileSelector(t testing.TB, selector string) *typesv1.ProfileType {
	ps, err := phlaremodel.ParseProfileTypeSelector(selector)
	require.NoError(t, err)
	return ps
}

func ingestProfiles(b testing.TB, db *Head, generator func(tsNano int64, t testing.TB) (*profilev1.Profile, string), from, to int64, step time.Duration, externalLabels ...*typesv1.LabelPair) {
	b.Helper()
	for i := from; i <= to; i += int64(step) {
		p, name := generator(i, b)
		db.Ingest(
			p, uuid.New(), append(externalLabels, &typesv1.LabelPair{Name: model.MetricNameLabel, Value: name}))
	}
}

var cpuProfileGenerator = func(tsNano int64, t testing.TB) (*profilev1.Profile, string) {
	p := parseProfile(t, testdataPrefix+"/testdata/profile")
	p.TimeNanos = tsNano
	return p, "process_cpu"
}
