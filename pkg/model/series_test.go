package model

import (
	"testing"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/testhelper"
)

func Test_SeriesMerger(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*typesv1.Series
		out  []*typesv1.Series
	}{
		{
			name: "empty",
			in:   [][]*typesv1.Series{},
			out:  []*typesv1.Series(nil),
		},
		{
			name: "merge two series",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1.Point{{Timestamp: 2, Value: 2}}},
				},
			},
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
			},
		},
		{
			name: "merge multiple series",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1.Point{{Timestamp: 2, Value: 2}}},
					{Labels: LabelsFromStrings("foor", "buzz"), Points: []*typesv1.Point{{Timestamp: 3, Value: 3}}},
				},
			},
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foor", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
				{Labels: LabelsFromStrings("foor", "buzz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 3, Value: 3}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, SumSeries(tc.in...))
		})
	}
}

func Test_SeriesMerger_Overlap(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*typesv1.Series
		out  []*typesv1.Series
	}{
		{
			name: "merge deduplicate overlapping series",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
				},
			},
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
				{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewSeriesMerger(false)
			for _, s := range tc.in {
				m.MergeSeries(s)
			}
			testhelper.EqualProto(t, tc.out, m.Series())
		})
	}
}
