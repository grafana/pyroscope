package querier

import (
	"context"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/clientpool"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

var foobarlabels = phlaremodel.Labels([]*typesv1.LabelPair{{Name: "foo", Value: "bar"}})

func TestSelectMergeStacktraces(t *testing.T) {
	resp1 := newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 1},
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	})
	resp2 := newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	})
	resp3 := newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 5},
			},
		},
	})
	res, err := selectMergeTree(context.Background(), []ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]{
		{
			response: resp1,
		},
		{
			response: resp2,
		},
		{
			response: resp3,
		},
	})
	require.NoError(t, err)
	requireFakeMergeProfilesStacktracesResultTree(t, res)
	all := []testProfile{}
	all = append(all, resp1.kept...)
	all = append(all, resp2.kept...)
	all = append(all, resp3.kept...)
	sort.Slice(all, func(i, j int) bool { return all[i].Ts < all[j].Ts })
	testhelper.EqualProto(t, all, []testProfile{
		{Ts: 1, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 2, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 3, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 4, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 5, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 6, Labels: &typesv1.Labels{Labels: foobarlabels}},
	})
	res, err = selectMergeTree(context.Background(), []ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]{
		{
			response: newFakeBidiClientStacktraces([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
					Profiles: []*ingestv1.SeriesProfile{
						{LabelIndex: 0, Timestamp: 1},
						{LabelIndex: 0, Timestamp: 2},
						{LabelIndex: 0, Timestamp: 4},
					},
				},
				{
					LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
					Profiles: []*ingestv1.SeriesProfile{
						{LabelIndex: 0, Timestamp: 5},
						{LabelIndex: 0, Timestamp: 6},
					},
				},
			}),
		},
	})
	require.NoError(t, err)
	requireFakeMergeProfilesStacktracesResultTree(t, res)
}

func TestSelectMergeStacktracesWithBlockDeduplication(t *testing.T) {
}

func TestSelectMergeByLabels(t *testing.T) {
	resp1 := newFakeBidiClientSeries([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 1},
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	}, &typesv1.Series{
		Labels: []*typesv1.LabelPair{{Name: "foo", Value: "bar"}},
		Points: []*typesv1.Point{{Timestamp: 1, Value: 1.0}, {Timestamp: 2, Value: 2.0}},
	})
	resp2 := newFakeBidiClientSeries([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	}, &typesv1.Series{
		Labels: foobarlabels,
		Points: []*typesv1.Point{{Timestamp: 3, Value: 3.0}, {Timestamp: 4, Value: 4.0}},
	})
	resp3 := newFakeBidiClientSeries([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 5},
			},
		},
	}, &typesv1.Series{
		Labels: foobarlabels,
		Points: []*typesv1.Point{{Timestamp: 5, Value: 5.0}, {Timestamp: 6, Value: 6.0}},
	})

	res, err := selectMergeSeries(context.Background(), nil, []ResponseFromReplica[clientpool.BidiClientMergeProfilesLabels]{
		{
			response: resp1,
		},
		{
			response: resp2,
		},
		{
			response: resp3,
		},
	})
	require.NoError(t, err)
	// ensure we have correctly selected the right profiles
	all := []testProfile{}
	all = append(all, resp1.kept...)
	all = append(all, resp2.kept...)
	all = append(all, resp3.kept...)
	sort.Slice(all, func(i, j int) bool { return all[i].Ts < all[j].Ts })
	testhelper.EqualProto(t, all, []testProfile{
		{Ts: 1, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 2, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 3, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 4, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 5, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 6, Labels: &typesv1.Labels{Labels: foobarlabels}},
	})
	values, err := iter.Slice(res)
	require.NoError(t, err)
	require.Equal(t, []phlaremodel.TimeSeriesValue{
		{Ts: 1, Value: 1.0, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
		{Ts: 2, Value: 2.0, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
		{Ts: 3, Value: 3.0, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
		{Ts: 4, Value: 4.0, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
		{Ts: 5, Value: 5.0, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
		{Ts: 6, Value: 6.0, Lbs: foobarlabels, LabelsHash: foobarlabels.Hash()},
	}, values)
}

func TestSelectMergePprof(t *testing.T) {
	resp1 := newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 1},
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	})
	resp2 := newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 2},
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 4},
			},
		},
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 5},
				{LabelIndex: 0, Timestamp: 6},
			},
		},
	})
	resp3 := newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
		{
			LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
			Profiles: []*ingestv1.SeriesProfile{
				{LabelIndex: 0, Timestamp: 3},
				{LabelIndex: 0, Timestamp: 5},
			},
		},
	})
	res, err := selectMergePprofProfile(context.Background(), &typesv1.ProfileType{}, []ResponseFromReplica[clientpool.BidiClientMergeProfilesPprof]{
		{
			response: resp1,
		},
		{
			response: resp2,
		},
		{
			response: resp3,
		},
	})
	require.NoError(t, err)
	requireFakeMergeProfilesPprof(t, 3, res)
	all := []testProfile{}
	all = append(all, resp1.kept...)
	all = append(all, resp2.kept...)
	all = append(all, resp3.kept...)
	sort.Slice(all, func(i, j int) bool { return all[i].Ts < all[j].Ts })
	testhelper.EqualProto(t, all, []testProfile{
		{Ts: 1, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 2, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 3, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 4, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 5, Labels: &typesv1.Labels{Labels: foobarlabels}},
		{Ts: 6, Labels: &typesv1.Labels{Labels: foobarlabels}},
	})
	res, err = selectMergePprofProfile(context.Background(), &typesv1.ProfileType{}, []ResponseFromReplica[clientpool.BidiClientMergeProfilesPprof]{
		{
			response: newFakeBidiClientProfiles([]*ingestv1.ProfileSets{
				{
					LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
					Profiles: []*ingestv1.SeriesProfile{
						{LabelIndex: 0, Timestamp: 1},
						{LabelIndex: 0, Timestamp: 2},
						{LabelIndex: 0, Timestamp: 4},
					},
				},
				{
					LabelsSets: []*typesv1.Labels{{Labels: foobarlabels}},
					Profiles: []*ingestv1.SeriesProfile{
						{LabelIndex: 0, Timestamp: 5},
						{LabelIndex: 0, Timestamp: 6},
					},
				},
			}),
		},
	})
	require.NoError(t, err)
	requireFakeMergeProfilesPprof(t, 1, res)
}

func BenchmarkSelectMergeStacktraces(b *testing.B) {
	rf := 3
	clientsCount := 20
	profilesCount := 2048
	batchCount := 5
	seriesCount := 50
	// todo stacktraces := 1000

	responses := make([]ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces], clientsCount*rf)

	for clientId := 0; clientId < clientsCount; clientId++ {
		batches := make([]*ingestv1.ProfileSets, batchCount)
		for batchID := 0; batchID < batchCount; batchID++ {
			batches[batchID] = &ingestv1.ProfileSets{
				LabelsSets: make([]*typesv1.Labels, seriesCount),
				Profiles:   make([]*ingestv1.SeriesProfile, profilesCount*seriesCount),
			}
			batch := batches[batchID]
			for i := 0; i < seriesCount; i++ {
				batch.LabelsSets[i] = &typesv1.Labels{
					Labels: []*typesv1.LabelPair{
						{Name: "client", Value: strconv.Itoa(clientId)},
						{Name: "series", Value: strconv.Itoa(i)},
					},
				}
			}
			sort.Slice(batch.LabelsSets, func(i, j int) bool {
				return model.CompareLabelPairs(batch.LabelsSets[i].Labels, batch.LabelsSets[j].Labels) < 0
			})
			for j := 0; j < profilesCount; j++ {
				for i := 0; i < seriesCount; i++ {
					batch.Profiles[j+(i*profilesCount)] = &ingestv1.SeriesProfile{LabelIndex: int32(i), Timestamp: int64(j + (batchID * profilesCount))}
				}
			}
		}
		for replica := 0; replica < rf; replica++ {
			responses[replica+(clientId*rf)] = ResponseFromReplica[clientpool.BidiClientMergeProfilesStacktraces]{
				response: newFakeBidiClientStacktraces(batches),
			}
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := selectMergeTree(context.Background(), responses)
		if err != nil {
			b.Fatal(err)
		}
	}
}
