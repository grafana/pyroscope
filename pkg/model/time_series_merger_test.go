package model

import (
	"testing"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/testhelper"
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
			testhelper.EqualProto(t, tc.out, MergeSeries(nil, tc.in...))
		})
	}
}

func Test_SeriesMerger_Annotations(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*typesv1.Series
		out  []*typesv1.Series
	}{
		{
			name: "merge two distinct annotations",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     1,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value1"},
								},
							},
						},
					},
				},
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     2,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value2"},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("foo", "bar"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1,
							Value:     3,
							Annotations: []*typesv1.ProfileAnnotation{
								{Key: "key1", Value: "value1"},
								{Key: "key1", Value: "value2"},
							},
						},
					},
				},
			},
		},
		{
			name: "merge duplicate annotations",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     1,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value1"},
									{Key: "key2", Value: "value2"},
								},
							},
						},
					},
				},
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     2,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value1"},
									{Key: "key3", Value: "value3"},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("foo", "bar"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1,
							Value:     3,
							Annotations: []*typesv1.ProfileAnnotation{
								{Key: "key1", Value: "value1"},
								{Key: "key2", Value: "value2"},
								{Key: "key3", Value: "value3"},
							},
						},
					},
				},
			},
		},
		{
			name: "merge all duplicate annotations",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     1,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value1"},
									{Key: "key2", Value: "value2"},
								},
							},
						},
					},
				},
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     2,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value1"},
									{Key: "key2", Value: "value2"},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("foo", "bar"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1,
							Value:     3,
							Annotations: []*typesv1.ProfileAnnotation{
								{Key: "key1", Value: "value1"},
								{Key: "key2", Value: "value2"},
							},
						},
					},
				},
			},
		},
		{
			name: "annotations sorted by key then value",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     1,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "z", Value: "last"},
									{Key: "a", Value: "first"},
								},
							},
						},
					},
				},
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     2,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "m", Value: "middle"},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("foo", "bar"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1,
							Value:     3,
							Annotations: []*typesv1.ProfileAnnotation{
								{Key: "a", Value: "first"},
								{Key: "m", Value: "middle"},
								{Key: "z", Value: "last"},
							},
						},
					},
				},
			},
		},
		{
			name: "empty annotations on one side",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp:   1,
								Value:       1,
								Annotations: []*typesv1.ProfileAnnotation{},
							},
						},
					},
				},
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     2,
								Annotations: []*typesv1.ProfileAnnotation{
									{Key: "key1", Value: "value1"},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("foo", "bar"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1,
							Value:     3,
							Annotations: []*typesv1.ProfileAnnotation{
								{Key: "key1", Value: "value1"},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, MergeSeries(nil, tc.in...))
		})
	}
}

func Test_SeriesMerger_Overlap_Sum(t *testing.T) {
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
				{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 1}}},
				{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 1}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, MergeSeries(nil, tc.in...))
		})
	}
}

func Test_SeriesMerger_Top(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*typesv1.Series
		out  []*typesv1.Series
		top  int
	}{
		{
			name: "top == len",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
				},
			},
			top: 2,
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 2}}},
				{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 1}}},
			},
		},
		{
			name: "top < len",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
				},
			},
			top: 1,
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 2}}},
			},
		},
		{
			name: "top > len",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
				},
				{
					{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
				},
			},
			top: 3,
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foo", "baz"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 2}}},
				{Labels: LabelsFromStrings("foo", "bar"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}, {Timestamp: 3, Value: 1}}},
			},
		},
		{
			name: "order",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foo", "d"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "e"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "c"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "a"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
					{Labels: LabelsFromStrings("foo", "b"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 3}}},
				},
			},
			top: 4,
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foo", "b"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 3}}},
				{Labels: LabelsFromStrings("foo", "a"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
				{Labels: LabelsFromStrings("foo", "c"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
				{Labels: LabelsFromStrings("foo", "d"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
			},
		},
		{
			name: "k == 0",
			in: [][]*typesv1.Series{
				{
					{Labels: LabelsFromStrings("foo", "d"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "c"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
					{Labels: LabelsFromStrings("foo", "a"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
					{Labels: LabelsFromStrings("foo", "b"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 3}}},
				},
			},
			top: 0,
			out: []*typesv1.Series{
				{Labels: LabelsFromStrings("foo", "b"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 3}}},
				{Labels: LabelsFromStrings("foo", "a"), Points: []*typesv1.Point{{Timestamp: 2, Value: 1}, {Timestamp: 3, Value: 2}}},
				{Labels: LabelsFromStrings("foo", "c"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
				{Labels: LabelsFromStrings("foo", "d"), Points: []*typesv1.Point{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 1}}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewTimeSeriesMerger(true)
			for _, s := range tc.in {
				m.MergeTimeSeries(s)
			}
			testhelper.EqualProto(t, tc.out, m.Top(tc.top))
		})
	}
}

func Test_SeriesMerger_WithExemplars(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   [][]*typesv1.Series
		out  []*typesv1.Series
	}{
		{
			name: "merge keeps highest value exemplar per profile ID",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     10,
								Exemplars: []*typesv1.Exemplar{
									{ProfileId: "prof-1", Value: 100, Timestamp: 1},
								},
							},
						},
					},
				},
				{
					{
						Labels: LabelsFromStrings("foo", "bar"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1,
								Value:     20,
								Exemplars: []*typesv1.Exemplar{
									{ProfileId: "prof-1", Value: 500, Timestamp: 1},
									{ProfileId: "prof-2", Value: 200, Timestamp: 1},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("foo", "bar"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1,
							Value:     30,
							Exemplars: []*typesv1.Exemplar{
								{ProfileId: "prof-1", Value: 500, Timestamp: 1},
								{ProfileId: "prof-2", Value: 200, Timestamp: 1},
							},
						},
					},
				},
			},
		},
		{
			name: "merge preserves exemplar labels",
			in: [][]*typesv1.Series{
				{
					{
						Labels: LabelsFromStrings("service_name", "api"),
						Points: []*typesv1.Point{
							{
								Timestamp: 1000,
								Value:     100,
								Exemplars: []*typesv1.Exemplar{
									{
										ProfileId: "prof-1",
										Value:     100,
										Timestamp: 1000,
										Labels:    []*typesv1.LabelPair{{Name: "pod", Value: "pod-123"}},
									},
								},
							},
						},
					},
				},
			},
			out: []*typesv1.Series{
				{
					Labels: LabelsFromStrings("service_name", "api"),
					Points: []*typesv1.Point{
						{
							Timestamp: 1000,
							Value:     100,
							Exemplars: []*typesv1.Exemplar{
								{
									ProfileId: "prof-1",
									Value:     100,
									Timestamp: 1000,
									Labels:    []*typesv1.LabelPair{{Name: "pod", Value: "pod-123"}},
								},
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testhelper.EqualProto(t, tc.out, MergeSeries(nil, tc.in...))
		})
	}
}
