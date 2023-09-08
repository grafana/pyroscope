// SPDX-License-Identifier: AGPL-3.0-only

package sharding

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseShard(t *testing.T) {
	tests := map[string]struct {
		input        string
		index, count uint64
		err          bool
	}{
		"should return error on invalid format": {
			input: "lsdjf",
			err:   true,
		},
		"should return error on invalid index (not an integer)": {
			input: "a_of_3",
			err:   true,
		},
		"should return error on invalid index (not positive)": {
			input: "-1_of_3",
			err:   true,
		},
		"should return error on invalid count (not positive)": {
			input: "-1_of_-3",
			err:   true,
		},
		"should return error on invalid index (too large)": {
			input: "4_of_3",
			err:   true,
		},
		"should return error on invalid index (too small)": {
			input: "0_of_3",
			err:   true,
		},
		"should return error on invalid separator": {
			input: "1_out_3",
			err:   true,
		},
		"should succeed on valid first shard ID": {
			input: "1_of_2",
			index: 0, // 0-based
			count: 2,
		},
		"should succeed on valid last shard selector": {
			input: "2_of_2",
			index: 1, // 0-based
			count: 2,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			index, count, err := ParseShardIDLabelValue(testData.input)
			if testData.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testData.index, index)
				require.Equal(t, testData.count, count)
			}
		})
	}
}

func TestRemoveShardFromMatchers(t *testing.T) {
	tests := map[string]struct {
		input            []*labels.Matcher
		expectedShard    *ShardSelector
		expectedMatchers []*labels.Matcher
		expectedError    error
	}{
		"should return no shard on empty label matchers": {},
		"should return no shard on no shard label matcher": {
			input: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test"),
				labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"),
			},
			expectedShard: nil,
			expectedMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test"),
				labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"),
			},
		},
		"should return matching shard and filter out its matcher": {
			input: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test"),
				labels.MustNewMatcher(labels.MatchEqual, ShardLabel, ShardSelector{ShardIndex: 1, ShardCount: 8}.LabelValue()),
				labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"),
			},
			expectedShard: &ShardSelector{
				ShardIndex: 1,
				ShardCount: 8,
			},
			expectedMatchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test"),
				labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"),
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			actualShard, actualMatchers, actualError := RemoveShardFromMatchers(testData.input)
			assert.Equal(t, testData.expectedShard, actualShard)
			assert.Equal(t, testData.expectedError, actualError)

			// Assert same matchers. We do some optimizations in mimir-prometheus which make
			// the label matchers not comparable with reflect.DeepEqual() so we're going to
			// compare their string representation.
			require.Len(t, actualMatchers, len(testData.expectedMatchers))
			for i := 0; i < len(testData.expectedMatchers); i++ {
				assert.Equal(t, testData.expectedMatchers[i].String(), actualMatchers[i].String())
			}
		})
	}
}

func TestShardFromMatchers(t *testing.T) {
	testExpr := []struct {
		input []*labels.Matcher
		shard *ShardSelector
		idx   int
		err   bool
	}{
		{
			input: []*labels.Matcher{
				{},
				{
					Name: ShardLabel,
					Type: labels.MatchEqual,
					Value: ShardSelector{
						ShardIndex: 10,
						ShardCount: 16,
					}.LabelValue(),
				},
				{},
			},
			shard: &ShardSelector{
				ShardIndex: 10,
				ShardCount: 16,
			},
			idx: 1,
			err: false,
		},
		{
			input: []*labels.Matcher{
				{
					Name:  ShardLabel,
					Type:  labels.MatchEqual,
					Value: "invalid-fmt",
				},
			},
			shard: nil,
			idx:   0,
			err:   true,
		},
		{
			input: []*labels.Matcher{},
			shard: nil,
			idx:   0,
			err:   false,
		},
	}

	for i, c := range testExpr {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			shard, idx, err := ShardFromMatchers(c.input)
			if c.err {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
				require.Equal(t, c.shard, shard)
				require.Equal(t, c.idx, idx)
			}
		})
	}
}

func TestFormatAndParseShardId(t *testing.T) {
	r := rand.New(rand.NewSource(0))

	const maxTests = 1000
	const maxShardCount = 10000

	for i := 0; i < maxTests; i++ {
		count := 1 + r.Intn(maxShardCount)
		id := r.Intn(count)

		require.True(t, id < count)

		out := FormatShardIDLabelValue(uint64(id), uint64(count))
		nid, ncount, err := ParseShardIDLabelValue(out)

		require.NoError(t, err)
		require.Equal(t, uint64(id), nid)
		require.Equal(t, uint64(count), ncount)
	}
}
