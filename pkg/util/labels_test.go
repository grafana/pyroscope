package util

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestMergeLabelNames(t *testing.T) {
	tests := []struct {
		Name      string
		Responses []*typesv1.LabelNamesResponse
		Want      *typesv1.LabelNamesResponse
	}{
		{
			Name:      "no_responses",
			Responses: []*typesv1.LabelNamesResponse{},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{},
				EstimatedCardinality: []int64{},
			},
		},
		{
			Name: "single_response",
			Responses: []*typesv1.LabelNamesResponse{
				{
					Names:                []string{"label_a", "label_b"},
					EstimatedCardinality: []int64{},
				},
			},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{"label_a", "label_b"},
				EstimatedCardinality: nil,
			},
		},
		{
			Name: "single_response_with_cardinality",
			Responses: []*typesv1.LabelNamesResponse{
				{
					Names:                []string{"label_a", "label_b"},
					EstimatedCardinality: []int64{10, 20},
				},
			},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{"label_a", "label_b"},
				EstimatedCardinality: []int64{10, 20},
			},
		},
		{
			Name: "multiple_responses",
			Responses: []*typesv1.LabelNamesResponse{
				{
					Names:                []string{"label_a", "label_b"},
					EstimatedCardinality: []int64{},
				},
				{
					Names:                []string{"label_b", "label_c"},
					EstimatedCardinality: []int64{},
				},
			},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{"label_a", "label_b", "label_c"},
				EstimatedCardinality: nil,
			},
		},
		{
			Name: "multiple_responses_with_cardinality",
			Responses: []*typesv1.LabelNamesResponse{
				{
					Names:                []string{"label_a", "label_b"},
					EstimatedCardinality: []int64{10, 20},
				},
				{
					Names:                []string{"label_b", "label_c"},
					EstimatedCardinality: []int64{5, 6},
				},
			},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{"label_a", "label_b", "label_c"},
				EstimatedCardinality: []int64{10, 25, 6},
			},
		},
		{
			Name: "response_missing_cardinality",
			Responses: []*typesv1.LabelNamesResponse{
				{
					Names:                []string{"label_a", "label_b"},
					EstimatedCardinality: []int64{10, 20},
				},
				{
					Names:                []string{"label_b", "label_c"},
					EstimatedCardinality: []int64{},
				},
			},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{"label_a", "label_b", "label_c"},
				EstimatedCardinality: nil,
			},
		},
		{
			Name: "multiple_responses_with_empty_response",
			Responses: []*typesv1.LabelNamesResponse{
				{
					Names:                []string{"label_a", "label_b"},
					EstimatedCardinality: []int64{10, 20},
				},
				{
					Names:                []string{},
					EstimatedCardinality: []int64{},
				},
			},
			Want: &typesv1.LabelNamesResponse{
				Names:                []string{"label_a", "label_b"},
				EstimatedCardinality: []int64{10, 20},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := MergeLabelNames(tt.Responses)
			require.Equal(t, tt.Want.Names, got.Names)
			require.Equal(t, tt.Want.EstimatedCardinality, got.EstimatedCardinality)
		})
	}
}
