package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
)

func TestStackTraceMerger(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       []*ingestv1.MergeProfilesStacktracesResult
		maxNodes int64
		expected *Tree
	}{
		{
			name:     "empty",
			in:       []*ingestv1.MergeProfilesStacktracesResult{},
			expected: newTree(nil),
		},
		{
			name: "single",
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
					},
					FunctionNames: []string{"my", "other", "stack"},
				},
			},
			expected: newTree([]stacktraces{
				{locations: []string{"other", "my"}, value: 1},
				{locations: []string{"stack", "other", "my"}, value: 3},
			}),
		},
		{
			name: "multiple",
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       4,
						},
					},
					FunctionNames: []string{"my", "other", "stack", "foo"},
				},
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       5,
						},
					},
					FunctionNames: []string{"my", "other", "stack", "bar"},
				},
			},
			expected: newTree([]stacktraces{
				{locations: []string{"bar"}, value: 5},
				{locations: []string{"foo"}, value: 4},
				{locations: []string{"other", "my"}, value: 2},
				{locations: []string{"stack", "other", "my"}, value: 6},
			}),
		},
		{
			name:     "multiple with truncation",
			maxNodes: 3,
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       4,
						},
					},
					FunctionNames: []string{"my", "qux", "stack", "foo"},
				},
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{1, 0},
							Value:       1,
						},
						{
							FunctionIds: []int32{2, 1, 0},
							Value:       3,
						},
						{
							FunctionIds: []int32{3},
							Value:       5,
						},
					},
					FunctionNames: []string{"my", "qux", "stack", "bar"},
				},
			},
			expected: newTree([]stacktraces{
				{locations: []string{"other"}, value: 9},
				{locations: []string{"qux", "my"}, value: 2},
				{locations: []string{"stack", "qux", "my"}, value: 6},
			}),
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := NewStackTraceMerger()
			for _, x := range tc.in {
				m.MergeStackTraces(x.Stacktraces, x.FunctionNames)
			}
			yn := m.TreeBytes(tc.maxNodes)
			actual, err := UnmarshalTree(yn)
			require.NoError(t, err)
			require.Equal(t, tc.expected.String(), actual.String())
		})
	}
}
