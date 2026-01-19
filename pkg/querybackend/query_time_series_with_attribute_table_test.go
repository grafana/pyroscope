package querybackend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestTimeSeriesWithAttributeTableAggregator(t *testing.T) {
	query := &queryv1.Query{
		TimeSeriesWithAttributeTable: &queryv1.TimeSeriesQuery{
			GroupBy: []string{"service_name"},
			Step:    1.0,
		},
	}

	req := &queryv1.InvokeRequest{
		StartTime: 1000,
		EndTime:   2000,
		Query:     []*queryv1.Query{query},
	}

	agg := newTimeSeriesWithAttributeTableAggregator(req).(*timeSeriesWithAttributeTableAggregator)

	// Report 1: 3 attributes (pod, version, region) at timestamp 1000
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"pod", "version", "region"},
		Values: []string{"a", "1.0", "us-east-1"},
	}
	report1 := &queryv1.Report{
		TimeSeriesWithAttributeTable: &queryv1.TimeSeriesWithAttributeTableReport{
			Query: query.TimeSeriesWithAttributeTable,
			TimeSeries: []*queryv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*queryv1.Point{{
					Timestamp: 1000,
					Value:     100,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-1",
						Value:         100,
						Timestamp:     1000,
						AttributeRefs: []int64{0, 1, 2},
					}},
				}},
			}},
			AttributeTable: table1,
		},
	}

	// Report 2: 2 attributes (pod, version) at timestamp 2000 - different structure!
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"pod", "version"},
		Values: []string{"b", "1.0"},
	}
	report2 := &queryv1.Report{
		TimeSeriesWithAttributeTable: &queryv1.TimeSeriesWithAttributeTableReport{
			Query: query.TimeSeriesWithAttributeTable,
			TimeSeries: []*queryv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*queryv1.Point{{
					Timestamp: 2000,
					Value:     200,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-2",
						Value:         200,
						Timestamp:     2000,
						AttributeRefs: []int64{0, 1},
					}},
				}},
			}},
			AttributeTable: table2,
		},
	}

	// Report 3: Same as report1 to test string interning - duplicate labels!
	table3 := &queryv1.AttributeTable{
		Keys:   []string{"pod", "version", "region"},
		Values: []string{"a", "1.0", "us-east-1"},
	}
	report3 := &queryv1.Report{
		TimeSeriesWithAttributeTable: &queryv1.TimeSeriesWithAttributeTableReport{
			Query: query.TimeSeriesWithAttributeTable,
			TimeSeries: []*queryv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*queryv1.Point{{
					Timestamp: 1000,
					Value:     50,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-3",
						Value:         50,
						Timestamp:     1000,
						AttributeRefs: []int64{0, 1, 2},
					}},
				}},
			}},
			AttributeTable: table3,
		},
	}

	// Aggregate all three reports
	err := agg.aggregate(report1)
	require.NoError(t, err)
	err = agg.aggregate(report2)
	require.NoError(t, err)
	err = agg.aggregate(report3)
	require.NoError(t, err)

	result := agg.build()
	require.NotNil(t, result.TimeSeriesWithAttributeTable)
	require.NotNil(t, result.TimeSeriesWithAttributeTable.AttributeTable)
	require.Len(t, result.TimeSeriesWithAttributeTable.TimeSeries, 1)

	series := result.TimeSeriesWithAttributeTable.TimeSeries[0]
	require.Len(t, series.Points, 2, "Should have 2 points (different timestamps)")

	attrTable := result.TimeSeriesWithAttributeTable.AttributeTable

	attrMap := make(map[int64]struct {
		key   string
		value string
	})
	for i := range attrTable.Keys {
		attrMap[int64(i)] = struct {
			key   string
			value string
		}{attrTable.Keys[i], attrTable.Values[i]}
	}

	// Point 1: timestamp 1000 - should keep prof-1 (value=100) not prof-3 (value=50)
	point1 := series.Points[0]
	require.Equal(t, int64(1000), point1.Timestamp)
	require.Len(t, point1.Exemplars, 1, "RangeSeries limits to 1 exemplar per point")
	ex1 := point1.Exemplars[0]
	assert.Equal(t, "prof-1", ex1.ProfileId, "Should keep highest value exemplar at timestamp 1000")
	assert.Equal(t, uint64(100), ex1.Value)
	require.Len(t, ex1.AttributeRefs, 3)

	// Verify prof-1 attributes: pod=a, version=1.0, region=us-east-1
	ex1Labels := make(map[string]string)
	for _, ref := range ex1.AttributeRefs {
		attr := attrMap[ref]
		ex1Labels[attr.key] = attr.value
	}
	assert.Equal(t, "a", ex1Labels["pod"])
	assert.Equal(t, "1.0", ex1Labels["version"])
	assert.Equal(t, "us-east-1", ex1Labels["region"])

	// Point 2: timestamp 2000 - should have prof-2
	point2 := series.Points[1]
	require.Equal(t, int64(2000), point2.Timestamp)
	require.Len(t, point2.Exemplars, 1)
	ex2 := point2.Exemplars[0]
	assert.Equal(t, "prof-2", ex2.ProfileId)
	assert.Equal(t, uint64(200), ex2.Value)
	require.Len(t, ex2.AttributeRefs, 2)

	// Verify prof-2 attributes: pod=b, version=1.0 (no region)
	ex2Labels := make(map[string]string)
	for _, ref := range ex2.AttributeRefs {
		attr := attrMap[ref]
		ex2Labels[attr.key] = attr.value
	}
	assert.Equal(t, "b", ex2Labels["pod"])
	assert.Equal(t, "1.0", ex2Labels["version"])
	_, hasRegion := ex2Labels["region"]
	assert.False(t, hasRegion, "prof-2 should not have region label")

	// Verify string interning: 4 unique key-value pairs total
	// - pod: a, b (2 entries)
	// - version: 1.0 (1 entry, shared by both exemplars - string interning!)
	// - region: us-east-1 (1 entry)
	assert.Len(t, attrTable.Keys, 4)
	assert.Len(t, attrTable.Values, 4)

	// Verify the actual contents
	allKeys := make(map[string][]string)
	for i := range attrTable.Keys {
		key := attrTable.Keys[i]
		value := attrTable.Values[i]
		allKeys[key] = append(allKeys[key], value)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, allKeys["pod"])
	assert.ElementsMatch(t, []string{"1.0"}, allKeys["version"])
	assert.ElementsMatch(t, []string{"us-east-1"}, allKeys["region"])
}
