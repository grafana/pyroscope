package model

import (
	"testing"

	typesv1alpha1 "github.com/grafana/phlare/api/gen/proto/go/types/v1alpha1"
	"github.com/grafana/phlare/pkg/testhelper"
)

func TestMergeSeries(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*typesv1alpha1.Series
		out  []*typesv1alpha1.Series
	}{
		{
			name: "empty",
			in:   [][]*typesv1alpha1.Series{},
			out:  []*typesv1alpha1.Series(nil),
		},
		{
			name: "merge two series",
			in: [][]*typesv1alpha1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1alpha1.Point{{Timestamp: 1, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1alpha1.Point{{Timestamp: 2, Value: 2}}},
				},
			},
			out: []*typesv1alpha1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1alpha1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
			},
		},
		{
			name: "merge multiple series",
			in: [][]*typesv1alpha1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1alpha1.Point{{Timestamp: 1, Value: 1}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*typesv1alpha1.Point{{Timestamp: 1, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1alpha1.Point{{Timestamp: 2, Value: 2}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*typesv1alpha1.Point{{Timestamp: 3, Value: 3}}},
				},
			},
			out: []*typesv1alpha1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1alpha1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
				{Labels: LabelsFromStrings("foor", "buzz"), Points: []*typesv1alpha1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 3, Value: 3}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, MergeSeries(tc.in...))
		})
	}
}
