package pprof

import (
	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/grafana/pyroscope/pkg/og/ingestion"
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
		"qwe.py:242 - main;qwe.py:50 - func1 10",
		"qwe.py:242 - main;qwe.py:8 - func2 13",
	}
	assert.Equal(t, expected, collapsed)

	assert.Equal(t, "qwe.py:242 - main", functionNameFromLocation(profile.Location[0].Id))
	assert.Equal(t, "qwe.py:50 - func1", functionNameFromLocation(profile.Location[1].Id))
	assert.Equal(t, "qwe.py:8 - func2", functionNameFromLocation(profile.Location[2].Id))
}
