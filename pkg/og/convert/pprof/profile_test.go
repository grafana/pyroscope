package pprof

import (
	"testing"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/api/model/labelset"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/pprof"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyPPROF(t *testing.T) {

	p := RawProfile{
		FormDataContentType: "multipart/form-data; boundary=ae798a53dec9077a712cf18e2ebf54842f5c792cfed6a31b6f469cfd2684",
		RawData: []byte("--ae798a53dec9077a712cf18e2ebf54842f5c792cfed6a31b6f469cfd2684\r\n" +
			"Content-Disposition: form-data; name=\"profile\"; filename=\"profile.pprof\"\r\n" +
			"Content-Type: application/octet-stream\r\n" +
			"\r\n" +
			"\r\n" +
			"--ae798a53dec9077a712cf18e2ebf54842f5c792cfed6a31b6f469cfd2684--\r\n"),
	}
	profile, err := p.ParseToPprof(nil, ingestion.Metadata{})
	require.NoError(t, err)
	require.Equal(t, 0, len(profile.Series))
}

func TestFixFunctionNamesForScriptingLanguages(t *testing.T) {
	profile := pprof.RawFromProto(&profilev1.Profile{
		StringTable: []string{"", "main", "func1", "func2", "qwe.py"},
		Function: []*profilev1.Function{
			{Id: 1, Name: 1, Filename: 4, SystemName: 1, StartLine: 239},
			{Id: 2, Name: 2, Filename: 4, SystemName: 2, StartLine: 42},
			{Id: 3, Name: 3, Filename: 4, SystemName: 3, StartLine: 7},
		},
		Location: []*profilev1.Location{
			{Id: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 242}}},
			{Id: 2, Line: []*profilev1.Line{{FunctionId: 2, Line: 50}}},
			{Id: 3, Line: []*profilev1.Line{{FunctionId: 3, Line: 8}}},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{10, 1000}},
			{LocationId: []uint64{3, 1}, Value: []int64{13, 1300}},
		},
	})
	functionNameFromLocation := func(locID uint64) string {
		for _, loc := range profile.Location {
			if loc.Id == locID {
				for _, fun := range profile.Function {
					if fun.Id == loc.Line[0].FunctionId {
						return profile.StringTable[fun.Name]
					}
				}
			}
		}
		return ""
	}

	md := ingestion.Metadata{
		SpyName: "pyspy",
	}

	FixFunctionNamesForScriptingLanguages(profile, md)

	assert.Len(t, profile.Function, 6) // we do not remove unreferenced functions for now, let the distributor do it
	assert.Len(t, profile.Location, 3)
	assert.Len(t, profile.Sample, 2)

	collapsed := bench.StackCollapseProto(profile.Profile, 0, 1)
	expected := []string{
		"qwe.py main;qwe.py func1 10",
		"qwe.py main;qwe.py func2 13",
	}
	assert.Equal(t, expected, collapsed)

	assert.Equal(t, "qwe.py main", functionNameFromLocation(profile.Location[0].Id))
	assert.Equal(t, "qwe.py func1", functionNameFromLocation(profile.Location[1].Id))
	assert.Equal(t, "qwe.py func2", functionNameFromLocation(profile.Location[2].Id))
}

func TestCreateLabels(t *testing.T) {
	testCases := []struct {
		name                string
		labelMap            map[string]string
		expectedServiceName string
	}{
		{
			name: "with existing service_name",
			labelMap: map[string]string{
				"service_name": "existing-service",
				"region":       "us-west",
			},
			expectedServiceName: "existing-service",
		},
		{
			name: "without service_name uses __name__ value",
			labelMap: map[string]string{
				"region":   "us-west",
				"__name__": "test-service",
			},
			expectedServiceName: "test-service",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := RawProfile{
				SampleTypeConfig: map[string]*tree.SampleTypeConfig{
					"samples": {
						DisplayName: "samples",
						Units:       "count",
					},
				},
			}

			// Create a proper pprof.Profile with sample types
			profile := &pprof.Profile{
				Profile: &profilev1.Profile{
					SampleType: []*profilev1.ValueType{
						{
							Type: 1,
							Unit: 2,
						},
					},
					StringTable: []string{"", "samples", "count"},
				},
			}

			// Create metadata with LabelSet
			md := ingestion.Metadata{
				LabelSet: labelset.New(tc.labelMap),
				SpyName:  "test-spy",
			}

			// Call createLabels
			labels := p.createLabels(profile, md)

			// Convert labels to a map for easier checking
			labelMap := make(map[string]string)
			for _, label := range labels {
				labelMap[label.Name] = label.Value
			}

			// Check that service_name has the expected value
			assert.Equal(t, tc.expectedServiceName, labelMap["service_name"], "service_name should have the expected value")

			// Check that required labels are present
			assert.Contains(t, labelMap, "__name__", "Should contain __name__ label")
			assert.Contains(t, labelMap, phlaremodel.LabelNameDelta, "Should contain delta label")
			assert.Contains(t, labelMap, phlaremodel.LabelNamePyroscopeSpy, "Should contain spy label")
		})
	}
}
