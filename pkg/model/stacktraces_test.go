package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/pkg/testhelper"
)

func TestMergeBatchResponse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       []*ingestv1.MergeProfilesStacktracesResult
		expected *ingestv1.MergeProfilesStacktracesResult
	}{
		{
			name: "empty",
			in:   []*ingestv1.MergeProfilesStacktracesResult{},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces:   nil,
				FunctionNames: nil,
			},
		},
		{
			name: "single",
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{0, 1},
							Value:       1,
						},
						{
							FunctionIds: []int32{0, 1, 2},
							Value:       3,
						},
					},
					FunctionNames: []string{"my", "other", "stack"},
				},
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       1,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       3,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
		{
			name: "multiple",
			in: []*ingestv1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1.StacktraceSample{
						{
							FunctionIds: []int32{0, 1},
							Value:       1,
						},
						{
							FunctionIds: []int32{0, 1, 2},
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
							FunctionIds: []int32{0, 1},
							Value:       1,
						},
						{
							FunctionIds: []int32{0, 1, 2},
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
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{4},
						Value:       5,
					},
					{
						FunctionIds: []int32{3},
						Value:       4,
					},
					{
						FunctionIds: []int32{0, 1},
						Value:       2,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       6,
					},
				},
				FunctionNames: []string{"my", "other", "stack", "foo", "bar"},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res := MergeBatchMergeStacktraces(tc.in...)
			testhelper.EqualProto(t, tc.expected, res)
		})
	}
}

func TestStackTraceMerger(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       []*ingestv1.MergeProfilesStacktracesResult
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
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := NewStackTraceMerger()
			for _, x := range tc.in {
				m.MergeStackTraces(x.Stacktraces, x.FunctionNames)
			}
			yn := m.TreeBytes(-1)
			actual, err := UnmarshalTree(yn)
			require.NoError(t, err)
			require.Equal(t, tc.expected.String(), actual.String())
		})
	}
}
