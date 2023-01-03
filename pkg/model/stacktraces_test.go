package model

import (
	"testing"

	ingestv1alpha1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1alpha1"
	"github.com/grafana/phlare/pkg/testhelper"
)

func TestMergeBatchResponse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       []*ingestv1alpha1.MergeProfilesStacktracesResult
		expected *ingestv1alpha1.MergeProfilesStacktracesResult
	}{
		{
			name: "empty",
			in:   []*ingestv1alpha1.MergeProfilesStacktracesResult{},
			expected: &ingestv1alpha1.MergeProfilesStacktracesResult{
				Stacktraces:   nil,
				FunctionNames: nil,
			},
		},
		{
			name: "single",
			in: []*ingestv1alpha1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1alpha1.StacktraceSample{
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
			expected: &ingestv1alpha1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1alpha1.StacktraceSample{
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
			in: []*ingestv1alpha1.MergeProfilesStacktracesResult{
				{
					Stacktraces: []*ingestv1alpha1.StacktraceSample{
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
					Stacktraces: []*ingestv1alpha1.StacktraceSample{
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
			expected: &ingestv1alpha1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1alpha1.StacktraceSample{
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
