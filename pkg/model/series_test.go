package model

import (
	"testing"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	"github.com/grafana/fire/pkg/testhelper"
)

func TestMergeSeries(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*commonv1.Series
		out  []*commonv1.Series
	}{
		{
			name: "empty",
			in:   [][]*commonv1.Series{},
			out:  []*commonv1.Series(nil),
		},
		{
			name: "merge two series",
			in: [][]*commonv1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{T: 1, V: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{T: 2, V: 2}}},
				},
			},
			out: []*commonv1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{T: 1, V: 1}, {T: 2, V: 2}}},
			},
		},
		{
			name: "merge multiple series",
			in: [][]*commonv1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{T: 1, V: 1}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*commonv1.Point{{T: 1, V: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{T: 2, V: 2}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*commonv1.Point{{T: 3, V: 3}}},
				},
			},
			out: []*commonv1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{T: 1, V: 1}, {T: 2, V: 2}}},
				{Labels: LabelsFromStrings("foor", "buzz"), Points: []*commonv1.Point{{T: 1, V: 1}, {T: 3, V: 3}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, MergeSeries(tc.in...))
		})
	}
}
