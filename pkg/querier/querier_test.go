package querier

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
	"github.com/grafana/pyroscope/pkg/pprof"
	pprofth "github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/grafana/pyroscope/pkg/storegateway"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/testhelper"
	"github.com/grafana/pyroscope/pkg/util"
)

type poolFactory struct {
	f func(addr string) (client.PoolClient, error)
}

func (p poolFactory) FromInstance(desc ring.InstanceDesc) (client.PoolClient, error) {
	return p.f(desc.Addr)
}

func Test_QuerySampleType(t *testing.T) {
	querier, err := New(&NewQuerierParams{
		Cfg: Config{
			PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
		},
		IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
			{Addr: "1"},
			{Addr: "2"},
			{Addr: "3"},
		}, 3),
		PoolFactory: &poolFactory{f: func(addr string) (client.PoolClient, error) {
			q := newFakeQuerier()
			switch addr {
			case "1":
				q.On("LabelValues", mock.Anything, mock.Anything).
					Return(connect.NewResponse(&typesv1.LabelValuesResponse{
						Names: []string{
							"foo::::",
							"bar::::",
						},
					}), nil)
			case "2":
				q.On("LabelValues", mock.Anything, mock.Anything).
					Return(connect.NewResponse(&typesv1.LabelValuesResponse{
						Names: []string{
							"bar::::",
							"buzz::::",
						},
					}), nil)
			case "3":
				q.On("LabelValues", mock.Anything, mock.Anything).
					Return(connect.NewResponse(&typesv1.LabelValuesResponse{
						Names: []string{
							"buzz::::",
							"foo::::",
						},
					}), nil)
			}
			return q, nil
		}},
		Logger: log.NewLogfmtLogger(os.Stdout),
	})
	require.NoError(t, err)

	out, err := querier.ProfileTypes(context.Background(), connect.NewRequest(&querierv1.ProfileTypesRequest{}))
	require.NoError(t, err)

	ids := make([]string, 0, len(out.Msg.ProfileTypes))
	for _, pt := range out.Msg.ProfileTypes {
		ids = append(ids, pt.ID)
	}
	require.NoError(t, err)
	require.Equal(t, []string{"bar::::", "buzz::::", "foo::::"}, ids)
}

func Test_QueryLabelValues(t *testing.T) {
	req := connect.NewRequest(&typesv1.LabelValuesRequest{Name: "foo"})
	querier, err := New(&NewQuerierParams{
		Cfg: Config{
			PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
		},
		IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
			{Addr: "1"},
			{Addr: "2"},
			{Addr: "3"},
		}, 3),
		PoolFactory: &poolFactory{f: func(addr string) (client.PoolClient, error) {
			q := newFakeQuerier()
			switch addr {
			case "1":
				q.On("LabelValues", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.LabelValuesResponse{Names: []string{"foo", "bar"}}), nil)
			case "2":
				q.On("LabelValues", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.LabelValuesResponse{Names: []string{"bar", "buzz"}}), nil)
			case "3":
				q.On("LabelValues", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.LabelValuesResponse{Names: []string{"buzz", "foo"}}), nil)
			}
			return q, nil
		}},
		Logger: log.NewLogfmtLogger(os.Stdout),
	})

	require.NoError(t, err)
	out, err := querier.LabelValues(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "buzz", "foo"}, out.Msg.Names)
}

func Test_QueryLabelNames(t *testing.T) {
	req := connect.NewRequest(&typesv1.LabelNamesRequest{})
	querier, err := New(&NewQuerierParams{
		Cfg: Config{
			PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
		},
		IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
			{Addr: "1"},
			{Addr: "2"},
			{Addr: "3"},
		}, 3),
		PoolFactory: &poolFactory{f: func(addr string) (client.PoolClient, error) {
			q := newFakeQuerier()
			switch addr {
			case "1":
				q.On("LabelNames", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"foo", "bar"}}), nil)
			case "2":
				q.On("LabelNames", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"bar", "buzz"}}), nil)
			case "3":
				q.On("LabelNames", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.LabelNamesResponse{Names: []string{"buzz", "foo"}}), nil)
			}
			return q, nil
		}},
		Logger: log.NewLogfmtLogger(os.Stdout),
	})

	require.NoError(t, err)
	out, err := querier.LabelNames(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "buzz", "foo"}, out.Msg.Names)
}

func Test_Series(t *testing.T) {
	foobarlabels := phlaremodel.NewLabelsBuilder(nil).Set("foo", "bar")
	foobuzzlabels := phlaremodel.NewLabelsBuilder(nil).Set("foo", "buzz")
	req := connect.NewRequest(&querierv1.SeriesRequest{Matchers: []string{`{foo="bar"}`}})
	ingesterResponse := connect.NewResponse(&ingestv1.SeriesResponse{LabelsSet: []*typesv1.Labels{
		{Labels: foobarlabels.Labels()},
		{Labels: foobuzzlabels.Labels()},
	}})
	querier, err := New(&NewQuerierParams{
		Cfg: Config{
			PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
		},
		IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
			{Addr: "1"},
			{Addr: "2"},
			{Addr: "3"},
		}, 3),
		PoolFactory: &poolFactory{func(addr string) (client.PoolClient, error) {
			q := newFakeQuerier()
			switch addr {
			case "1":
				q.On("Series", mock.Anything, mock.Anything).Return(ingesterResponse, nil)
			case "2":
				q.On("Series", mock.Anything, mock.Anything).Return(ingesterResponse, nil)
			case "3":
				q.On("Series", mock.Anything, mock.Anything).Return(ingesterResponse, nil)
			}
			return q, nil
		}},
		Logger: log.NewLogfmtLogger(os.Stdout),
	})

	require.NoError(t, err)
	out, err := querier.Series(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []*typesv1.Labels{
		{Labels: foobarlabels.Labels()},
		{Labels: foobuzzlabels.Labels()},
	}, out.Msg.LabelsSet)
}

func newBlockMeta(ulids ...string) *connect.Response[ingestv1.BlockMetadataResponse] {
	resp := &ingestv1.BlockMetadataResponse{}

	resp.Blocks = make([]*typesv1.BlockInfo, len(ulids))
	for i, ulid := range ulids {
		resp.Blocks[i] = &typesv1.BlockInfo{
			Ulid: ulid,
		}
	}

	return connect.NewResponse(resp)
}

var endpointNotExistingErr = connect.NewError(
	connect.CodeInternal,
	connect.NewError(
		connect.CodeUnknown,
		errors.New("405 Method Not Allowed"),
	),
)

func Test_isEndpointNotExisting(t *testing.T) {
	assert.False(t, isEndpointNotExistingErr(nil))
	assert.False(t, isEndpointNotExistingErr(errors.New("my-error")))
	assert.True(t, isEndpointNotExistingErr(endpointNotExistingErr))
}

func Test_SelectMergeStacktraces(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, tc := range []struct {
		blockSelect bool
		name        string
	}{
		// This tests the interoperability between older ingesters and new queriers
		{false, "WithoutBlockHints"},
		{true, "WithBlockHints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				LabelSelector: `{app="foo"}`,
				ProfileTypeID: "memory:inuse_space:bytes:space:byte",
				Start:         now + 0,
				End:           now + 2,
			})
			bidi1 := newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: now + 1, LabelIndex: 0},
						{Timestamp: now + 2, LabelIndex: 1},
						{Timestamp: now + 2, LabelIndex: 0},
					},
				},
			})
			bidi2 := newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: now + 1, LabelIndex: 1},
						{Timestamp: now + 1, LabelIndex: 0},
						{Timestamp: now + 2, LabelIndex: 1},
					},
				},
			})
			bidi3 := newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: now + 1, LabelIndex: 1},
						{Timestamp: now + 1, LabelIndex: 0},
						{Timestamp: now + 2, LabelIndex: 0},
					},
				},
			})
			querier, err := New(&NewQuerierParams{
				Cfg: Config{
					PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
				},
				IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
					{Addr: "1"},
					{Addr: "2"},
					{Addr: "3"},
				}, 3),
				PoolFactory: &poolFactory{func(addr string) (client.PoolClient, error) {
					q := newFakeQuerier()
					switch addr {
					case "1":
						q.mockMergeStacktraces(bidi1, []string{"a", "d"}, tc.blockSelect)
					case "2":
						q.mockMergeStacktraces(bidi2, []string{"b", "d"}, tc.blockSelect)
					case "3":
						q.mockMergeStacktraces(bidi3, []string{"c", "d"}, tc.blockSelect)
					}
					return q, nil
				}},
				Logger: log.NewLogfmtLogger(os.Stdout),
			})
			require.NoError(t, err)
			flame, err := querier.SelectMergeStacktraces(context.Background(), req)
			require.NoError(t, err)

			sort.Strings(flame.Msg.Flamegraph.Names)
			require.Equal(t, []string{"bar", "buzz", "foo", "total"}, flame.Msg.Flamegraph.Names)
			require.Equal(t, []int64{0, 2, 0, 0}, flame.Msg.Flamegraph.Levels[0].Values)
			require.Equal(t, int64(2), flame.Msg.Flamegraph.Total)
			require.Equal(t, int64(2), flame.Msg.Flamegraph.MaxSelf)
			var selected []testProfile
			selected = append(selected, bidi1.kept...)
			selected = append(selected, bidi2.kept...)
			selected = append(selected, bidi3.kept...)
			sort.Slice(selected, func(i, j int) bool {
				if selected[i].Ts == selected[j].Ts {
					return phlaremodel.CompareLabelPairs(selected[i].Labels.Labels, selected[j].Labels.Labels) < 0
				}
				return selected[i].Ts < selected[j].Ts
			})
			require.Len(t, selected, 4)
			require.Equal(t,
				[]testProfile{
					{Ts: now + 1, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}}}},
					{Ts: now + 1, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}}}},
					{Ts: now + 2, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}}}},
					{Ts: now + 2, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}}}},
				}, selected)
		})
	}
}

func Test_SelectMergeProfiles(t *testing.T) {
	for _, tc := range []struct {
		blockSelect bool
		name        string
	}{
		// This tests the interoberabitlity between older ingesters and new queriers
		{false, "WithoutBlockHints"},
		{true, "WithBlockHints"},
	} {
		t.Run(tc.name, func(t *testing.T) {

			req := connect.NewRequest(&querierv1.SelectMergeProfileRequest{
				LabelSelector: `{app="foo"}`,
				ProfileTypeID: "memory:inuse_space:bytes:space:byte",
				Start:         0,
				End:           2,
			})
			bidi1 := newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: 1, LabelIndex: 0},
						{Timestamp: 2, LabelIndex: 1},
						{Timestamp: 2, LabelIndex: 0},
					},
				},
			})
			bidi2 := newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: 1, LabelIndex: 1},
						{Timestamp: 1, LabelIndex: 0},
						{Timestamp: 2, LabelIndex: 1},
					},
				},
			})
			bidi3 := newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: 1, LabelIndex: 1},
						{Timestamp: 1, LabelIndex: 0},
						{Timestamp: 2, LabelIndex: 0},
					},
				},
			})
			querier, err := New(&NewQuerierParams{
				Cfg: Config{
					PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
				},
				IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
					{Addr: "1"},
					{Addr: "2"},
					{Addr: "3"},
				}, 3),
				PoolFactory: &poolFactory{f: func(addr string) (client.PoolClient, error) {
					q := newFakeQuerier()
					switch addr {
					case "1":
						q.mockMergeProfile(bidi1, []string{"a", "d"}, tc.blockSelect)
					case "2":
						q.mockMergeProfile(bidi2, []string{"b", "d"}, tc.blockSelect)
					case "3":
						q.mockMergeProfile(bidi3, []string{"c", "d"}, tc.blockSelect)
					}
					switch addr {
					case "1":
						q.On("MergeProfilesPprof", mock.Anything).Once().Return(bidi1)
					case "2":
						q.On("MergeProfilesPprof", mock.Anything).Once().Return(bidi2)
					case "3":
						q.On("MergeProfilesPprof", mock.Anything).Once().Return(bidi3)
					}
					return q, nil
				}},
				Logger: log.NewLogfmtLogger(os.Stdout),
			})
			require.NoError(t, err)
			res, err := querier.SelectMergeProfile(context.Background(), req)
			require.NoError(t, err)
			require.NotNil(t, res)
			data, err := proto.Marshal(res.Msg)
			require.NoError(t, err)
			actual, err := profile.ParseUncompressed(data)
			require.NoError(t, err)

			expected := pprofth.FooBarProfile.Copy()
			expected.DurationNanos = model.Time(req.Msg.End).UnixNano() - model.Time(req.Msg.Start).UnixNano()
			expected.TimeNanos = model.Time(req.Msg.End).UnixNano()
			for _, s := range expected.Sample {
				s.Value[0] = s.Value[0] * 2
			}
			require.Equal(t, actual, expected)

			var selected []testProfile
			selected = append(selected, bidi1.kept...)
			selected = append(selected, bidi2.kept...)
			selected = append(selected, bidi3.kept...)
			sort.Slice(selected, func(i, j int) bool {
				if selected[i].Ts == selected[j].Ts {
					return phlaremodel.CompareLabelPairs(selected[i].Labels.Labels, selected[j].Labels.Labels) < 0
				}
				return selected[i].Ts < selected[j].Ts
			})
			require.Len(t, selected, 4)
			require.Equal(t,
				[]testProfile{
					{Ts: 1, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}}}},
					{Ts: 1, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}}}},
					{Ts: 2, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}}}},
					{Ts: 2, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}}}},
				}, selected)
		})
	}
}

func TestSelectSeries(t *testing.T) {
	// now := time.Now().UnixMilli()
	for _, tc := range []struct {
		blockSelect bool
		name        string
	}{
		// This tests the interoberabitlity between older ingesters and new queriers
		{false, "WithoutBlockHints"},
		{true, "WithBlockHints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&querierv1.SelectSeriesRequest{
				LabelSelector: `{app="foo"}`,
				ProfileTypeID: "memory:inuse_space:bytes:space:byte",
				Start:         0,
				End:           2,
				Step:          0.001,
			})
			bidi1 := newFakeBidiClientSeries([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: 1, LabelIndex: 0},
						{Timestamp: 2, LabelIndex: 1},
						{Timestamp: 2, LabelIndex: 0},
					},
				},
			}, &typesv1.Series{Labels: foobarlabels, Points: []*typesv1.Point{{Value: 1, Timestamp: 1}, {Value: 2, Timestamp: 2}}})
			bidi2 := newFakeBidiClientSeries([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: 1, LabelIndex: 1},
						{Timestamp: 1, LabelIndex: 0},
						{Timestamp: 2, LabelIndex: 1},
					},
				},
			}, &typesv1.Series{Labels: foobarlabels, Points: []*typesv1.Point{{Value: 1, Timestamp: 1}, {Value: 2, Timestamp: 2}}})
			bidi3 := newFakeBidiClientSeries([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}},
						},
						{
							Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}},
						},
					},
					Profiles: []*ingestv1.SeriesProfile{
						{Timestamp: 1, LabelIndex: 1},
						{Timestamp: 1, LabelIndex: 0},
						{Timestamp: 2, LabelIndex: 0},
					},
				},
			}, &typesv1.Series{Labels: foobarlabels, Points: []*typesv1.Point{{Value: 1, Timestamp: 1}, {Value: 2, Timestamp: 2}}})
			querier, err := New(&NewQuerierParams{
				Cfg: Config{
					PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
				},
				IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
					{Addr: "1"},
					{Addr: "2"},
					{Addr: "3"},
				}, 3),
				PoolFactory: &poolFactory{f: func(addr string) (client.PoolClient, error) {
					q := newFakeQuerier()
					switch addr {
					case "1":
						q.mockMergeLabels(bidi1, []string{"a", "d"}, tc.blockSelect)
					case "2":
						q.mockMergeLabels(bidi2, []string{"b", "d"}, tc.blockSelect)
					case "3":
						q.mockMergeLabels(bidi3, []string{"c", "d"}, tc.blockSelect)
					}
					return q, nil
				}},
				Logger: log.NewLogfmtLogger(os.Stdout),
			})
			require.NoError(t, err)
			res, err := querier.SelectSeries(context.Background(), req)
			require.NoError(t, err)
			// Only 2 results are used since the 3rd not required because of replication.
			testhelper.EqualProto(t, []*typesv1.Series{
				{Labels: foobarlabels, Points: []*typesv1.Point{{Value: 2, Timestamp: 1}, {Value: 4, Timestamp: 2}}},
			}, res.Msg.Series)
			var selected []testProfile
			selected = append(selected, bidi1.kept...)
			selected = append(selected, bidi2.kept...)
			selected = append(selected, bidi3.kept...)
			sort.Slice(selected, func(i, j int) bool {
				if selected[i].Ts == selected[j].Ts {
					return phlaremodel.CompareLabelPairs(selected[i].Labels.Labels, selected[j].Labels.Labels) < 0
				}
				return selected[i].Ts < selected[j].Ts
			})
			require.Len(t, selected, 4)
			require.Equal(t,
				[]testProfile{
					{Ts: 1, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}}}},
					{Ts: 1, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}}}},
					{Ts: 2, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "bar"}}}},
					{Ts: 2, Labels: &typesv1.Labels{Labels: []*typesv1.LabelPair{{Name: "app", Value: "foo"}}}},
				}, selected)
		})
	}
}

type fakeQuerierIngester struct {
	mock.Mock
	testhelper.FakePoolClient
}

func newFakeQuerier() *fakeQuerierIngester {
	return &fakeQuerierIngester{}
}

func (f *fakeQuerierIngester) mockMergeStacktraces(bidi *fakeBidiClientStacktraces, blocks []string, blockSelect bool) {
	if blockSelect {
		f.On("BlockMetadata", mock.Anything, mock.Anything).Once().Return(newBlockMeta(blocks...), nil)
	} else {
		f.On("BlockMetadata", mock.Anything, mock.Anything).Once().Return(nil, endpointNotExistingErr)
	}
	f.On("MergeProfilesStacktraces", mock.Anything).Once().Return(bidi)
}

func (f *fakeQuerierIngester) mockMergeLabels(bidi *fakeBidiClientSeries, blocks []string, blockSelect bool) {
	if blockSelect {
		f.On("BlockMetadata", mock.Anything, mock.Anything).Once().Return(newBlockMeta(blocks...), nil)
	} else {
		f.On("BlockMetadata", mock.Anything, mock.Anything).Once().Return(nil, endpointNotExistingErr)
	}
	f.On("MergeProfilesLabels", mock.Anything).Once().Return(bidi)
}

func (f *fakeQuerierIngester) mockMergeProfile(bidi *fakeBidiClientProfiles, blocks []string, blockSelect bool) {
	if blockSelect {
		f.On("BlockMetadata", mock.Anything, mock.Anything).Once().Return(newBlockMeta(blocks...), nil)
	} else {
		f.On("BlockMetadata", mock.Anything, mock.Anything).Once().Return(nil, endpointNotExistingErr)
	}
	f.On("MergeProfilesPprof", mock.Anything).Once().Return(bidi)
}

func (f *fakeQuerierIngester) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[typesv1.LabelValuesResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[typesv1.LabelValuesResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}
	return res, err
}

func (f *fakeQuerierIngester) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[typesv1.LabelNamesResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[typesv1.LabelNamesResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}
	return res, err
}

func (f *fakeQuerierIngester) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[ingestv1.ProfileTypesResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[ingestv1.ProfileTypesResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}

	return res, err
}

func (f *fakeQuerierIngester) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[ingestv1.SeriesResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[ingestv1.SeriesResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}

	return res, err
}

func (f *fakeQuerierIngester) BlockMetadata(ctx context.Context, req *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[ingestv1.BlockMetadataResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[ingestv1.BlockMetadataResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}

	return res, err
}

func (f *fakeQuerierIngester) GetProfileStats(ctx context.Context, req *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[typesv1.GetProfileStatsResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[typesv1.GetProfileStatsResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}

	return res, err
}

func (f *fakeQuerierIngester) GetBlockStats(ctx context.Context, req *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error) {
	var (
		args = f.Called(ctx, req)
		res  *connect.Response[ingestv1.GetBlockStatsResponse]
		err  error
	)
	if args[0] != nil {
		res = args[0].(*connect.Response[ingestv1.GetBlockStatsResponse])
	}
	if args[1] != nil {
		err = args.Get(1).(error)
	}

	return res, err
}

type testProfile struct {
	Ts     int64
	Labels *typesv1.Labels
}

type fakeBidiClientStacktraces struct {
	profiles chan *ingestv1.ProfileSets
	batches  []*ingestv1.ProfileSets
	kept     []testProfile
	cur      *ingestv1.ProfileSets
}

func newFakeBidiClientStacktraces(batches []*ingestv1.ProfileSets) *fakeBidiClientStacktraces {
	res := &fakeBidiClientStacktraces{
		profiles: make(chan *ingestv1.ProfileSets, 1),
	}
	res.profiles <- batches[0]
	batches = batches[1:]
	res.batches = batches
	return res
}

func (f *fakeBidiClientStacktraces) Send(in *ingestv1.MergeProfilesStacktracesRequest) error {
	if in.Request != nil {
		return nil
	}
	for i, b := range in.Profiles {
		if b {
			f.kept = append(f.kept, testProfile{
				Ts:     f.cur.Profiles[i].Timestamp,
				Labels: f.cur.LabelsSets[f.cur.Profiles[i].LabelIndex],
			})
		}
	}
	if len(f.batches) == 0 {
		close(f.profiles)
		return nil
	}
	f.profiles <- f.batches[0]
	f.batches = f.batches[1:]
	return nil
}

func (f *fakeBidiClientStacktraces) Receive() (*ingestv1.MergeProfilesStacktracesResponse, error) {
	profiles := <-f.profiles
	if profiles == nil {
		return &ingestv1.MergeProfilesStacktracesResponse{
			Result: &ingestv1.MergeProfilesStacktracesResult{
				Format: ingestv1.StacktracesMergeFormat_MERGE_FORMAT_STACKTRACES,
				Stacktraces: []*ingestv1.StacktraceSample{
					{FunctionIds: []int32{0, 1, 2}, Value: 1},
				},
				FunctionNames: []string{"foo", "bar", "buzz"},
			},
		}, nil
	}
	f.cur = profiles
	return &ingestv1.MergeProfilesStacktracesResponse{
		SelectedProfiles: profiles,
	}, nil
}
func (f *fakeBidiClientStacktraces) CloseRequest() error  { return nil }
func (f *fakeBidiClientStacktraces) CloseResponse() error { return nil }

func requireFakeMergeProfilesStacktracesResultTree(t *testing.T, r *phlaremodel.Tree) {
	flame := phlaremodel.NewFlameGraph(r, -1)
	sort.Strings(flame.Names)
	require.Equal(t, []string{"bar", "buzz", "foo", "total"}, flame.Names)
}

type fakeBidiClientProfiles struct {
	profiles chan *ingestv1.ProfileSets
	batches  []*ingestv1.ProfileSets
	kept     []testProfile
	cur      *ingestv1.ProfileSets
}

func newFakeBidiClientProfiles(batches []*ingestv1.ProfileSets) *fakeBidiClientProfiles {
	res := &fakeBidiClientProfiles{
		profiles: make(chan *ingestv1.ProfileSets, 1),
	}
	res.profiles <- batches[0]
	batches = batches[1:]
	res.batches = batches
	return res
}

func (f *fakeBidiClientProfiles) Send(in *ingestv1.MergeProfilesPprofRequest) error {
	if in.Request != nil {
		return nil
	}
	for i, b := range in.Profiles {
		if b {
			f.kept = append(f.kept, testProfile{
				Ts:     f.cur.Profiles[i].Timestamp,
				Labels: f.cur.LabelsSets[f.cur.Profiles[i].LabelIndex],
			})
		}
	}
	if len(f.batches) == 0 {
		close(f.profiles)
		return nil
	}
	f.profiles <- f.batches[0]
	f.batches = f.batches[1:]
	return nil
}

func (f *fakeBidiClientProfiles) Receive() (*ingestv1.MergeProfilesPprofResponse, error) {
	profiles := <-f.profiles
	if profiles == nil {
		var buf bytes.Buffer
		if err := pprofth.FooBarProfile.WriteUncompressed(&buf); err != nil {
			return nil, err
		}
		return &ingestv1.MergeProfilesPprofResponse{
			Result: buf.Bytes(),
		}, nil
	}
	f.cur = profiles
	return &ingestv1.MergeProfilesPprofResponse{
		SelectedProfiles: profiles,
	}, nil
}
func (f *fakeBidiClientProfiles) CloseRequest() error  { return nil }
func (f *fakeBidiClientProfiles) CloseResponse() error { return nil }

func requireFakeMergeProfilesPprof(t *testing.T, n int64, r *profilev1.Profile) {
	x, err := pprof.FromProfile(pprofth.FooBarProfile)
	for _, s := range x.Sample {
		s.Value[0] *= n
	}
	x.DurationNanos *= n
	require.NoError(t, err)
	require.Equal(t, x, r)
}

type fakeBidiClientSeries struct {
	profiles chan *ingestv1.ProfileSets
	batches  []*ingestv1.ProfileSets
	kept     []testProfile
	cur      *ingestv1.ProfileSets

	result []*typesv1.Series
}

func newFakeBidiClientSeries(batches []*ingestv1.ProfileSets, result ...*typesv1.Series) *fakeBidiClientSeries {
	res := &fakeBidiClientSeries{
		profiles: make(chan *ingestv1.ProfileSets, 1),
	}
	res.profiles <- batches[0]
	batches = batches[1:]
	res.batches = batches
	res.result = result
	return res
}

func (f *fakeBidiClientSeries) Send(in *ingestv1.MergeProfilesLabelsRequest) error {
	if in.Request != nil {
		return nil
	}
	for i, b := range in.Profiles {
		if b {
			f.kept = append(f.kept, testProfile{
				Ts:     f.cur.Profiles[i].Timestamp,
				Labels: f.cur.LabelsSets[f.cur.Profiles[i].LabelIndex],
			})
		}
	}
	if len(f.batches) == 0 {
		close(f.profiles)
		return nil
	}
	f.profiles <- f.batches[0]
	f.batches = f.batches[1:]
	return nil
}

func (f *fakeBidiClientSeries) Receive() (*ingestv1.MergeProfilesLabelsResponse, error) {
	profiles := <-f.profiles
	if profiles == nil {
		return &ingestv1.MergeProfilesLabelsResponse{
			Series: f.result,
		}, nil
	}
	f.cur = profiles
	return &ingestv1.MergeProfilesLabelsResponse{
		SelectedProfiles: profiles,
	}, nil
}
func (f *fakeBidiClientSeries) CloseRequest() error  { return nil }
func (f *fakeBidiClientSeries) CloseResponse() error { return nil }

func (f *fakeQuerierIngester) MergeSpanProfile(ctx context.Context) clientpool.BidiClientMergeSpanProfile {
	var (
		args = f.Called(ctx)
		res  clientpool.BidiClientMergeSpanProfile
	)
	if args[0] != nil {
		res = args[0].(clientpool.BidiClientMergeSpanProfile)
	}

	return res
}

func (f *fakeQuerierIngester) MergeProfilesStacktraces(ctx context.Context) clientpool.BidiClientMergeProfilesStacktraces {
	var (
		args = f.Called(ctx)
		res  clientpool.BidiClientMergeProfilesStacktraces
	)
	if args[0] != nil {
		res = args[0].(clientpool.BidiClientMergeProfilesStacktraces)
	}

	return res
}

func (f *fakeQuerierIngester) MergeProfilesLabels(ctx context.Context) clientpool.BidiClientMergeProfilesLabels {
	var (
		args = f.Called(ctx)
		res  clientpool.BidiClientMergeProfilesLabels
	)
	if args[0] != nil {
		res = args[0].(clientpool.BidiClientMergeProfilesLabels)
	}

	return res
}

func (f *fakeQuerierIngester) MergeProfilesPprof(ctx context.Context) clientpool.BidiClientMergeProfilesPprof {
	var (
		args = f.Called(ctx)
		res  clientpool.BidiClientMergeProfilesPprof
	)
	if args[0] != nil {
		res = args[0].(clientpool.BidiClientMergeProfilesPprof)
	}

	return res
}

func Test_RangeSeriesSum(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []ProfileValue
		out  []*typesv1.Series
	}{
		{
			name: "single series",
			in: []ProfileValue{
				{Ts: 1, Value: 1},
				{Ts: 1, Value: 1},
				{Ts: 2, Value: 2},
				{Ts: 3, Value: 3},
				{Ts: 4, Value: 4},
				{Ts: 5, Value: 5},
			},
			out: []*typesv1.Series{
				{
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 2},
						{Timestamp: 2, Value: 2},
						{Timestamp: 3, Value: 3},
						{Timestamp: 4, Value: 4},
						{Timestamp: 5, Value: 5},
					},
				},
			},
		},
		{
			name: "multiple series",
			in: []ProfileValue{
				{Ts: 1, Value: 1, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 1, Value: 1, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 2, Value: 1, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 3, Value: 1, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 3, Value: 1, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 4, Value: 4, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 4, Value: 4, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 4, Value: 4, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 5, Value: 5, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
			},
			out: []*typesv1.Series{
				{
					Labels: foobarlabels,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1},
						{Timestamp: 2, Value: 1},
						{Timestamp: 4, Value: 4},
						{Timestamp: 5, Value: 5},
					},
				},
				{
					Labels: foobuzzlabels,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1},
						{Timestamp: 3, Value: 2},
						{Timestamp: 4, Value: 8},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := iter.NewSliceIterator(tc.in)
			out := rangeSeries(in, 1, 5, 1, nil)
			testhelper.EqualProto(t, tc.out, out)
		})
	}
}

func Test_RangeSeriesAvg(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []ProfileValue
		out  []*typesv1.Series
	}{
		{
			name: "single series",
			in: []ProfileValue{
				{Ts: 1, Value: 1},
				{Ts: 1, Value: 2},
				{Ts: 2, Value: 2},
				{Ts: 2, Value: 3},
				{Ts: 3, Value: 4},
				{Ts: 4, Value: 5},
			},
			out: []*typesv1.Series{
				{
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1.5}, // avg of 1 and 2
						{Timestamp: 2, Value: 2.5}, // avg of 2 and 3
						{Timestamp: 3, Value: 4},
						{Timestamp: 4, Value: 5},
					},
				},
			},
		},
		{
			name: "multiple series",
			in: []ProfileValue{
				{Ts: 1, Value: 1, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 1, Value: 1, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 2, Value: 1, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 2, Value: 2, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 3, Value: 1, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 3, Value: 2, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 4, Value: 4, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 4, Value: 6, Lbs: foobuzzlabels, LabelsHash: foobuzzlabels.Hash()},
				{Ts: 4, Value: 4, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
				{Ts: 5, Value: 5, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
			},
			out: []*typesv1.Series{
				{
					Labels: foobarlabels,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1},
						{Timestamp: 2, Value: 1.5}, // avg of 1 and 2
						{Timestamp: 4, Value: 4},
						{Timestamp: 5, Value: 5},
					},
				},
				{
					Labels: foobuzzlabels,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1},
						{Timestamp: 3, Value: 1.5}, // avg of 1 and 2
						{Timestamp: 4, Value: 5},   // avg of 4 and 6
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := iter.NewSliceIterator(tc.in)
			aggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
			out := rangeSeries(in, 1, 5, 1, &aggregation)
			testhelper.EqualProto(t, tc.out, out)
		})
	}
}

func Test_splitQueryToStores(t *testing.T) {
	for _, tc := range []struct {
		name            string
		now             model.Time
		start, end      model.Time
		queryStoreAfter time.Duration
		plan            blockPlan

		expected storeQueries
	}{
		{
			// ----|-----|-----|----|----
			//     ^     ^     ^    ^
			//     cutoff now start end
			//
			name:            "start and end are in the future",
			now:             model.TimeFromUnixNano(0),
			start:           model.TimeFromUnixNano(int64(time.Hour)),
			end:             model.TimeFromUnixNano(int64(2 * time.Hour)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: false,
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(time.Hour)),
					end:         model.TimeFromUnixNano(int64(2 * time.Hour)),
				},
			},
		},
		{
			// ----|-------|-----|----|----
			//     ^       ^     ^    ^
			//     cutoff start now  end
			//
			name:            "end is in the future and start is after the cutoff",
			now:             model.TimeFromUnixNano(int64(time.Hour)),
			start:           model.TimeFromUnixNano(int64(45 * time.Minute)),
			end:             model.TimeFromUnixNano(int64(2 * time.Hour)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: false,
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(45 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(2 * time.Hour)),
				},
			},
		},
		{
			// ----|-------|-----|----|----
			//     ^       ^     ^    ^
			//     start  cutoff now  end
			//
			name:            "end is in the future and start is before the cutoff",
			now:             model.TimeFromUnixNano(int64(time.Hour)),
			start:           model.TimeFromUnixNano(int64(15 * time.Minute)),
			end:             model.TimeFromUnixNano(int64(2 * time.Hour)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(15 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(30 * time.Minute)),
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(30*time.Minute)) + 1,
					end:         model.TimeFromUnixNano(int64(2 * time.Hour)),
				},
			},
		},
		{
			// ----|-----|-----|----|----
			//     ^     ^     ^    ^
			//    start end cutoff now
			//
			name:            "start and end are in the past and cutoff is in the future",
			now:             model.TimeFromUnixNano(int64(2 * time.Hour)),
			start:           model.TimeFromUnixNano(0),
			end:             model.TimeFromUnixNano(int64(time.Hour)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(0),
					end:         model.TimeFromUnixNano(int64(time.Hour)),
				},
				ingester: storeQuery{
					shouldQuery: false,
				},
			},
		},
		{
			// ----|-----|-----|----|----
			//     ^     ^     ^    ^
			//    start cutoff end now
			//
			name:            "start and end are within cutoff",
			now:             model.TimeFromUnixNano(int64(1 * time.Hour)),
			start:           model.TimeFromUnixNano(0),
			end:             model.TimeFromUnixNano(int64(45 * time.Minute)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(0),
					end:         model.TimeFromUnixNano(int64(30 * time.Minute)),
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(30*time.Minute)) + 1,
					end:         model.TimeFromUnixNano(int64(45 * time.Minute)),
				},
			},
		},
		{
			// ----|----------|----|----
			//     ^          ^    ^
			//   start=cutoff end now
			//
			name:            "start is exactly at cutoff",
			now:             model.TimeFromUnixNano(int64(1 * time.Hour)),
			start:           model.TimeFromUnixNano(int64(30 * time.Minute)),
			end:             model.TimeFromUnixNano(int64(45 * time.Minute)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: false,
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(30 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(45 * time.Minute)),
				},
			},
		},
		{
			// ----|------|--------|----
			//     ^      ^        ^
			//   start end=cutoff now
			//
			name:            "end is exactly at cutoff",
			now:             model.TimeFromUnixNano(int64(15 * time.Hour)),
			start:           model.TimeFromUnixNano(int64(60 * time.Minute)),
			end:             model.TimeFromUnixNano(int64(30 * time.Minute)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(60 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(30 * time.Minute)),
				},
				ingester: storeQuery{
					shouldQuery: false,
				},
			},
		},
		{
			// ----|------|-----|----|----
			//     ^      ^     ^    ^
			//    cutoff start end  now
			//
			name:            "start is after at cutoff",
			now:             model.TimeFromUnixNano(int64(1 * time.Hour)),
			start:           model.TimeFromUnixNano(int64(30 * time.Minute)),
			end:             model.TimeFromUnixNano(int64(45 * time.Minute)),
			queryStoreAfter: 30 * time.Minute,

			expected: storeQueries{
				queryStoreAfter: 30 * time.Minute,
				storeGateway: storeQuery{
					shouldQuery: false,
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(30 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(45 * time.Minute)),
				},
			},
		},
		{
			name:            "with a plan we touch all stores at full time window and eleminate later based on the plan",
			now:             model.TimeFromUnixNano(int64(4 * time.Hour)),
			start:           model.TimeFromUnixNano(int64(30 * time.Minute)),
			end:             model.TimeFromUnixNano(int64(45*time.Minute) + int64(3*time.Hour)),
			queryStoreAfter: 30 * time.Minute,
			plan:            blockPlan{"replica-a": &blockPlanEntry{InstanceType: ingesterInstance, BlockHints: &ingestv1.BlockHints{Ulids: []string{"block-a", "block-b"}}}},

			expected: storeQueries{
				queryStoreAfter: 0,
				storeGateway: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(30 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(45*time.Minute) + int64(3*time.Hour)),
				},
				ingester: storeQuery{
					shouldQuery: true,
					start:       model.TimeFromUnixNano(int64(30 * time.Minute)),
					end:         model.TimeFromUnixNano(int64(45*time.Minute) + int64(3*time.Hour)),
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			actual := splitQueryToStores(
				tc.start,
				tc.end,
				tc.now,
				tc.queryStoreAfter,
				tc.plan,
			)
			require.Equal(t, tc.expected, actual)
		})
	}
}

func Test_GetProfileStats(t *testing.T) {
	ctx := tenant.InjectTenantID(context.Background(), "1234")

	dbPath := t.TempDir()
	localBucket, err := objstoreclient.NewBucket(ctx, objstoreclient.Config{
		StorageBackendConfig: objstoreclient.StorageBackendConfig{
			Backend: objstoreclient.Filesystem,
			Filesystem: filesystem.Config{
				Directory: dbPath,
			},
		},
		StoragePrefix: "testdata",
	}, "")
	require.NoError(t, err)

	index := bucketindex.Index{Blocks: []*bucketindex.Block{{
		MinTime: 0,
		MaxTime: 3,
	}},
		Version: bucketindex.IndexVersion3,
	}
	indexJson, err := json.Marshal(index)
	require.NoError(t, err)

	var gzipContent bytes.Buffer
	gzip := gzip.NewWriter(&gzipContent)
	gzip.Name = bucketindex.IndexFilename
	_, err = gzip.Write(indexJson)
	gzip.Close()
	require.NoError(t, err)

	err = localBucket.Upload(ctx, path.Join("1234", "phlaredb", bucketindex.IndexCompressedFilename), &gzipContent)
	require.NoError(t, err)

	req := connect.NewRequest(&typesv1.GetProfileStatsRequest{})
	querier, err := New(&NewQuerierParams{
		Cfg: Config{
			PoolConfig: clientpool.PoolConfig{ClientCleanupPeriod: 1 * time.Millisecond},
		},
		IngestersRing: testhelper.NewMockRing([]ring.InstanceDesc{
			{Addr: "1"},
			{Addr: "2"},
			{Addr: "3"},
		}, 3),
		PoolFactory: &poolFactory{f: func(addr string) (client.PoolClient, error) {
			q := newFakeQuerier()
			switch addr {
			case "1":
				q.On("GetProfileStats", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.GetProfileStatsResponse{
					DataIngested:      true,
					OldestProfileTime: 1,
					NewestProfileTime: 4,
				}), nil)
			case "2":
				q.On("GetProfileStats", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.GetProfileStatsResponse{
					DataIngested:      true,
					OldestProfileTime: 1,
					NewestProfileTime: 5,
				}), nil)
			case "3":
				q.On("GetProfileStats", mock.Anything, mock.Anything).Return(connect.NewResponse(&typesv1.GetProfileStatsResponse{
					DataIngested:      true,
					OldestProfileTime: 2,
					NewestProfileTime: 5,
				}), nil)
			}
			return q, nil
		}},
		Logger:        log.NewLogfmtLogger(os.Stdout),
		StorageBucket: localBucket,
		StoreGatewayCfg: storegateway.Config{
			ShardingRing: storegateway.RingConfig{
				Ring: util.CommonRingConfig{
					KVStore: kv.Config{
						Store: "inmemory",
					},
				},
				ReplicationFactor: 1,
			},
		},
	})

	require.NoError(t, err)
	out, err := querier.GetProfileStats(ctx, req)
	require.NoError(t, err)
	require.Equal(t, &typesv1.GetProfileStatsResponse{
		DataIngested:      true,
		OldestProfileTime: 0,
		NewestProfileTime: 5,
	}, out.Msg)
}

// The code below can be useful for testing deduping directly to a cluster.
// func TestDedupeLive(t *testing.T) {
// 	clients, err := createClients(context.Background())
// 	require.NoError(t, err)
// 	st, err := dedupe(context.Background(), clients)
// 	require.NoError(t, err)
// 	require.Equal(t, 2, len(st))
// }

// func createClients(ctx context.Context) ([]responseFromIngesters[BidiClientMergeProfilesStacktraces], error) {
// 	var clients []responseFromIngesters[BidiClientMergeProfilesStacktraces]
// 	for i := 1; i < 6; i++ {
// 		addr := fmt.Sprintf("localhost:4%d00", i)
// 		c, err := clientpool.PoolFactory(addr)
// 		if err != nil {
// 			return nil, err
// 		}
// 		res, err := c.Check(ctx, &grpc_health_v1.HealthCheckRequest{
// 			Service: ingestv1.IngesterService_ServiceDesc.ServiceName,
// 		})
// 		if err != nil {
// 			return nil, err
// 		}
// 		if res.Status != grpc_health_v1.HealthCheckResponse_SERVING {
// 			return nil, fmt.Errorf("ingester %s is not serving", addr)
// 		}
// 		bidi := c.(IngesterQueryClient).MergeProfilesStacktraces(ctx)
// 		profileType, err := phlaremodel.ParseProfileTypeSelector("process_cpu:cpu:nanoseconds:cpu:nanoseconds")
// 		if err != nil {
// 			return nil, err
// 		}
// 		now := time.Now()
// 		err = bidi.Send(&ingestv1.MergeProfilesStacktracesRequest{
// 			Request: &ingestv1.SelectProfilesRequest{
// 				LabelSelector: `{namespace="phlare-dev-001"}`,
// 				Type:          profileType,
// 				Start:         int64(model.TimeFromUnixNano(now.Add(-30 * time.Minute).UnixNano())),
// 				End:           int64(model.TimeFromUnixNano(now.UnixNano())),
// 			},
// 		})
// 		if err != nil {
// 			return nil, err
// 		}
// 		clients = append(clients, responseFromIngesters[BidiClientMergeProfilesStacktraces]{
// 			response: bidi,
// 			addr:     addr,
// 		})
// 	}
// 	return clients, nil
// }
