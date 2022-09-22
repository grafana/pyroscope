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
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{Timestamp: 1, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{Timestamp: 2, Value: 2}}},
				},
			},
			out: []*commonv1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
			},
		},
		{
			name: "merge multiple series",
			in: [][]*commonv1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{Timestamp: 1, Value: 1}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*commonv1.Point{{Timestamp: 1, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{Timestamp: 2, Value: 2}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*commonv1.Point{{Timestamp: 3, Value: 3}}},
				},
			},
			out: []*commonv1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*commonv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
				{Labels: LabelsFromStrings("foor", "buzz"), Points: []*commonv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 3, Value: 3}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, MergeSeries(tc.in...))
		})
	}
}
