package tsdb

import (
	"fmt"
	"sort"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/shard"
)

func Test_BitPrefixGetShards(t *testing.T) {
	for _, tt := range []struct {
		total    uint32
		filter   bool
		shard    *shard.Annotation
		expected []uint32
	}{
		// equal factors
		{16, false, &shard.Annotation{Shard: 0, Of: 16}, []uint32{0}},
		{16, false, &shard.Annotation{Shard: 4, Of: 16}, []uint32{4}},
		{16, false, &shard.Annotation{Shard: 15, Of: 16}, []uint32{15}},

		// idx factor a larger factor of 2
		{32, false, &shard.Annotation{Shard: 0, Of: 16}, []uint32{0, 1}},
		{32, false, &shard.Annotation{Shard: 4, Of: 16}, []uint32{8, 9}},
		{32, false, &shard.Annotation{Shard: 15, Of: 16}, []uint32{30, 31}},
		{64, false, &shard.Annotation{Shard: 15, Of: 16}, []uint32{60, 61, 62, 63}},

		// // idx factor a smaller factor of 2
		{8, true, &shard.Annotation{Shard: 0, Of: 16}, []uint32{0}},
		{8, true, &shard.Annotation{Shard: 4, Of: 16}, []uint32{2}},
		{8, true, &shard.Annotation{Shard: 15, Of: 16}, []uint32{7}},
	} {
		tt := tt
		t.Run(tt.shard.String()+fmt.Sprintf("_total_%d", tt.total), func(t *testing.T) {
			ii, err := NewBitPrefixWithShards(tt.total)
			require.Nil(t, err)
			res, filter := ii.getShards(tt.shard)
			resInt := []uint32{}
			for _, r := range res {
				resInt = append(resInt, r.shard)
			}
			require.Equal(t, tt.filter, filter)
			require.Equal(t, tt.expected, resInt)
		})
	}
}

func Test_BitPrefixValidateShards(t *testing.T) {
	ii, err := NewBitPrefixWithShards(32)
	require.Nil(t, err)
	require.NoError(t, ii.validateShard(&shard.Annotation{Shard: 1, Of: 16}))
	require.Error(t, ii.validateShard(&shard.Annotation{Shard: 1, Of: 15}))
}

func Test_BitPrefixCreation(t *testing.T) {
	// non factor of 2 shard factor
	_, err := NewBitPrefixWithShards(6)
	require.Error(t, err)

	// valid shard factor
	_, err = NewBitPrefixWithShards(4)
	require.Nil(t, err)
}

func Test_BitPrefixDeleteAddLoopkup(t *testing.T) {
	index, err := NewBitPrefixWithShards(DefaultIndexShards)
	require.Nil(t, err)
	lbs := []*typesv1.LabelPair{
		{Name: "foo", Value: "foo"},
		{Name: "bar", Value: "bar"},
		{Name: "buzz", Value: "buzz"},
	}
	sort.Sort(phlaremodel.Labels(lbs))

	index.Add(lbs, model.Fingerprint((phlaremodel.Labels(lbs).Hash())))
	index.Delete(lbs, model.Fingerprint(phlaremodel.Labels(lbs).Hash()))
	ids, err := index.Lookup([]*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "foo", "foo"),
	}, nil)
	require.NoError(t, err)
	require.Len(t, ids, 0)
}

func Test_BitPrefix_hash_mapping(t *testing.T) {
	lbs := []*typesv1.LabelPair{
		{Name: "compose_project", Value: "loki-boltdb-storage-s3"},
		{Name: "compose_service", Value: "ingester-2"},
		{Name: "container_name", Value: "loki-boltdb-storage-s3_ingester-2_1"},
		{Name: "filename", Value: "/var/log/docker/790fef4c6a587c3b386fe85c07e03f3a1613f4929ca3abaa4880e14caadb5ad1/json.log"},
		{Name: "host", Value: "docker-desktop"},
		{Name: "source", Value: "stderr"},
	}

	// for _, shard := range []uint32{2, 4, 8, 16, 32, 64, 128} {
	for _, shardID := range []uint32{2} {
		t.Run(fmt.Sprintf("%d", shardID), func(t *testing.T) {
			ii, err := NewBitPrefixWithShards(shardID)
			require.Nil(t, err)

			requestedFactor := 16

			fp := model.Fingerprint(phlaremodel.Labels(lbs).Hash())
			ii.Add(lbs, fp)

			requiredBits := index.NewShard(0, uint32(requestedFactor)).RequiredBits()
			expShard := uint32(phlaremodel.Labels(lbs).Hash() >> (64 - requiredBits))

			res, err := ii.Lookup(
				[]*labels.Matcher{{
					Type:  labels.MatchEqual,
					Name:  "compose_project",
					Value: "loki-boltdb-storage-s3",
				}},
				&shard.Annotation{
					Shard: int(expShard),
					Of:    requestedFactor,
				},
			)
			require.NoError(t, err)
			require.Len(t, res, 1)
			require.Equal(t, fp, res[0])
		})
	}
}

func Test_BitPrefixNoMatcherLookup(t *testing.T) {
	lbs := []*typesv1.LabelPair{
		{Name: "foo", Value: "bar"},
		{Name: "hi", Value: "hello"},
	}
	// with no shard param
	ii, err := NewBitPrefixWithShards(16)
	require.Nil(t, err)
	fp := model.Fingerprint(phlaremodel.Labels(lbs).Hash())
	ii.Add(lbs, fp)
	ids, err := ii.Lookup(nil, nil)
	require.Nil(t, err)
	require.Equal(t, fp, ids[0])

	// with shard param
	ii, err = NewBitPrefixWithShards(16)
	require.Nil(t, err)
	expShard := uint32(fp >> (64 - index.NewShard(0, 16).RequiredBits()))
	ii.Add(lbs, fp)
	ids, err = ii.Lookup(nil, &shard.Annotation{Shard: int(expShard), Of: 16})
	require.Nil(t, err)
	require.Equal(t, fp, ids[0])
}

func Test_BitPrefixConsistentMapping(t *testing.T) {
	a, err := NewBitPrefixWithShards(16)
	require.Nil(t, err)
	b, err := NewBitPrefixWithShards(32)
	require.Nil(t, err)

	for i := 0; i < 100; i++ {
		lbs := []*typesv1.LabelPair{
			{Name: "foo", Value: "bar"},
			{Name: "hi", Value: fmt.Sprint(i)},
		}

		fp := model.Fingerprint(phlaremodel.Labels(lbs).Hash())
		a.Add(lbs, fp)
		b.Add(lbs, fp)
	}

	shardMax := 8
	for i := 0; i < shardMax; i++ {
		shard := &shard.Annotation{
			Shard: i,
			Of:    shardMax,
		}

		aIDs, err := a.Lookup([]*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
		}, shard)
		require.Nil(t, err)

		bIDs, err := b.Lookup([]*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
		}, shard)
		require.Nil(t, err)

		sorter := func(xs []model.Fingerprint) {
			sort.Slice(xs, func(i, j int) bool {
				return xs[i] < xs[j]
			})
		}
		sorter(aIDs)
		sorter(bIDs)

		require.Equal(t, aIDs, bIDs, "incorrect shard mapping for shard %v", shard)
	}
}
