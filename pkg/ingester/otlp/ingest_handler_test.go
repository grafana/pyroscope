package otlp

import (
	"context"
	"flag"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/grafana/dskit/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1experimental2 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	v1experimental "go.opentelemetry.io/proto/otlp/profiles/v1development"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/strprofile"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockotlp"
)

func TestGetServiceNameFromAttributes(t *testing.T) {
	tests := []struct {
		name     string
		attrs    []*v1.KeyValue
		expected string
	}{
		{
			name:     "empty attributes",
			attrs:    []*v1.KeyValue{},
			expected: phlaremodel.AttrServiceNameFallback,
		},
		{
			name: "empty attributes",
			attrs: []*v1.KeyValue{
				{
					Key: "process.executable.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "bash",
						},
					},
				},
			},
			expected: phlaremodel.AttrServiceNameFallback + ":bash",
		},
		{
			name: "service name present",
			attrs: []*v1.KeyValue{
				{
					Key: "service.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "test-service",
						},
					},
				},
				{
					Key: "process.executable.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "test-executable",
						},
					},
				},
			},
			expected: "test-service",
		},
		{
			name: "service name empty",
			attrs: []*v1.KeyValue{
				{
					Key: "service.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "",
						},
					},
				},
				{
					Key: "process.executable.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "",
						},
					},
				},
			},
			expected: phlaremodel.AttrServiceNameFallback,
		},
		{
			name: "service name among other attributes",
			attrs: []*v1.KeyValue{
				{
					Key: "host.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "host1",
						},
					},
				},
				{
					Key: "service.name",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "test-service",
						},
					},
				},
			},
			expected: "test-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getServiceNameFromAttributes(tt.attrs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAttributesUnique(t *testing.T) {
	tests := []struct {
		name          string
		existingAttrs []*typesv1.LabelPair
		newAttrs      []*v1.KeyValue
		processedKeys map[string]bool
		expected      []*typesv1.LabelPair
	}{
		{
			name:          "empty attributes",
			existingAttrs: []*typesv1.LabelPair{},
			newAttrs:      []*v1.KeyValue{},
			processedKeys: make(map[string]bool),
			expected:      []*typesv1.LabelPair{},
		},
		{
			name: "new unique attributes",
			existingAttrs: []*typesv1.LabelPair{
				{Name: "existing", Value: "value"},
			},
			newAttrs: []*v1.KeyValue{
				{
					Key: "new",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "newvalue",
						},
					},
				},
			},
			processedKeys: map[string]bool{"existing": true},
			expected: []*typesv1.LabelPair{
				{Name: "existing", Value: "value"},
				{Name: "new", Value: "newvalue"},
			},
		},
		{
			name: "duplicate attributes",
			existingAttrs: []*typesv1.LabelPair{
				{Name: "key1", Value: "value1"},
			},
			newAttrs: []*v1.KeyValue{
				{
					Key: "key1",
					Value: &v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "value2",
						},
					},
				},
			},
			processedKeys: map[string]bool{"key1": true},
			expected: []*typesv1.LabelPair{
				{Name: "key1", Value: "value1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendAttributesUnique(tt.existingAttrs, tt.newAttrs, tt.processedKeys)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func readJSONFile(t *testing.T, filename string) string {
	data, err := os.ReadFile(filename)
	require.NoError(t, err, "filename: "+filename)
	return string(data)
}

func TestConversion(t *testing.T) {

	testdata := []struct {
		name             string
		expectedJsonFile string
		expectedError    string
		profile          func() *otlpbuilder
	}{
		{
			name:             "symbolized function names",
			expectedJsonFile: "testdata/TestSymbolizedFunctionNames.json",
			profile: func() *otlpbuilder {
				b := new(otlpbuilder)
				b.dictionary.MappingTable = []*v1experimental.Mapping{{
					MemoryStart:      0x1000,
					MemoryLimit:      0x1000,
					FilenameStrindex: b.addstr("file1.so"),
				}}
				b.dictionary.LocationTable = []*v1experimental.Location{{
					MappingIndex: 0,
					Address:      0x1e0,
					Line:         nil,
				}, {
					MappingIndex: 0,
					Address:      0x2f0,
					Line:         nil,
				}}
				b.dictionary.StackTable = []*v1experimental.Stack{{
					LocationIndices: []int32{0, 1},
				}}
				b.profile.SampleType = &v1experimental.ValueType{
					TypeStrindex: b.addstr("samples"),
					UnitStrindex: b.addstr("ms"),
				}
				b.profile.Sample = []*v1experimental.Sample{{
					StackIndex: 0,
					Values:     []int64{0xef},
				}}
				return b
			},
		},
		{
			name:             "offcpu",
			expectedJsonFile: "testdata/TestConversionOffCpu.json",
			profile: func() *otlpbuilder {
				b := new(otlpbuilder)
				b.profile.SampleType = &v1experimental.ValueType{
					TypeStrindex: b.addstr("events"),
					UnitStrindex: b.addstr("nanoseconds"),
				}
				b.dictionary.MappingTable = []*v1experimental.Mapping{{
					MemoryStart:      0x1000,
					MemoryLimit:      0x1000,
					FilenameStrindex: b.addstr("file1.so"),
				}}
				b.dictionary.LocationTable = []*v1experimental.Location{{
					MappingIndex: 0,
					Address:      0x1e0,
				}, {
					MappingIndex: 0,
					Address:      0x2f0,
				}, {
					MappingIndex: 0,
					Address:      0x3f0,
				}}
				b.dictionary.StackTable = []*v1experimental.Stack{{
					LocationIndices: []int32{0, 1},
				}, {
					LocationIndices: []int32{2},
				}}
				b.profile.Sample = []*v1experimental.Sample{{
					StackIndex: 0,
					Values:     []int64{0xef},
				}, {
					StackIndex: 1,
					Values:     []int64{1, 2, 3, 4, 5, 6},
				}}
				return b
			},
		},
		{
			name:          "samples with different value sizes ",
			expectedError: "sample values length mismatch",
			profile: func() *otlpbuilder {
				b := new(otlpbuilder)
				b.profile.SampleType = &v1experimental.ValueType{
					TypeStrindex: b.addstr("wrote_type"),
					UnitStrindex: b.addstr("wrong_unit"),
				}
				b.dictionary.MappingTable = []*v1experimental.Mapping{{
					MemoryStart:      0x1000,
					MemoryLimit:      0x1000,
					FilenameStrindex: b.addstr("file1.so"),
				}}
				b.dictionary.LocationTable = []*v1experimental.Location{{
					MappingIndex: 0,
					Address:      0x1e0,
				}, {
					MappingIndex: 0,
					Address:      0x2f0,
				}, {
					MappingIndex: 0,
					Address:      0x3f0,
				}}
				b.dictionary.StackTable = []*v1experimental.Stack{{
					LocationIndices: []int32{0, 1},
				}, {
					LocationIndices: []int32{2},
				}}
				b.profile.PeriodType = &v1experimental.ValueType{
					TypeStrindex:           b.addstr("period_type"),
					UnitStrindex:           b.addstr("period_unit"),
					AggregationTemporality: 0,
				}
				b.profile.Period = 100
				b.profile.Sample = []*v1experimental.Sample{{
					StackIndex: 0,
					Values:     []int64{0xef},
				}, {
					StackIndex: 1,
					Values:     []int64{1, 2, 3, 4, 5, 6}, // should be rejected because of that
				}}
				return b
			},
		},
	}

	for _, td := range testdata {
		td := td

		t.Run(td.name, func(t *testing.T) {
			svc := mockotlp.NewMockPushService(t)
			var profiles []*model.PushRequest
			svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				c := (args.Get(1)).(*model.PushRequest)
				profiles = append(profiles, c)
			}).Return(nil, nil).Maybe()
			b := td.profile()
			b.profile.TimeUnixNano = 239
			req := &v1experimental2.ExportProfilesServiceRequest{
				ResourceProfiles: []*v1experimental.ResourceProfiles{{
					ScopeProfiles: []*v1experimental.ScopeProfiles{{
						Profiles: []*v1experimental.Profile{
							&b.profile,
						}}}}},
				Dictionary: &b.dictionary}
			logger := test.NewTestingLogger(t)
			h := NewOTLPIngestHandler(testConfig(), svc, logger, false)
			_, err := h.Export(context.Background(), req)

			if td.expectedError == "" {
				require.NoError(t, err)
				require.Equal(t, 1, len(profiles))

				gp := profiles[0].Series[0].Profile.Profile

				jsonStr, err := strprofile.Stringify(gp, strprofile.Options{})
				assert.NoError(t, err)
				expectedJSON := readJSONFile(t, td.expectedJsonFile)
				assert.JSONEq(t, expectedJSON, jsonStr)
			} else {
				require.Error(t, err)
				require.True(t, strings.Contains(err.Error(), td.expectedError))
			}
		})
	}

}

func TestSampleAttributes(t *testing.T) {
	// Create a profile with two samples, with different sample attributes
	// one process=firefox, the other process=chrome
	// expect both of them to be present in the converted pprof as labels, but not series labels
	svc := mockotlp.NewMockPushService(t)
	var profiles []*model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		c := (args.Get(1)).(*model.PushRequest)
		profiles = append(profiles, c)
	}).Return(nil, nil)

	otlpb := new(otlpbuilder)
	otlpb.dictionary.MappingTable = []*v1experimental.Mapping{{
		MemoryStart:      0x1000,
		MemoryLimit:      0x1000,
		FilenameStrindex: otlpb.addstr("firefox.so"),
	}, {
		MemoryStart:      0x1000,
		MemoryLimit:      0x1000,
		FilenameStrindex: otlpb.addstr("chrome.so"),
	}}

	otlpb.dictionary.LocationTable = []*v1experimental.Location{{
		MappingIndex: 0,
		Address:      0x1e,
	}, {
		MappingIndex: 0,
		Address:      0x2e,
	}, {
		MappingIndex: 1,
		Address:      0x3e,
	}, {
		MappingIndex: 1,
		Address:      0x4e,
	}}
	otlpb.dictionary.StackTable = []*v1experimental.Stack{{
		LocationIndices: []int32{0, 1},
	}, {
		LocationIndices: []int32{2, 3},
	}}
	otlpb.profile.Sample = []*v1experimental.Sample{{
		StackIndex:       0,
		Values:           []int64{0xef},
		AttributeIndices: []int32{0},
	}, {
		StackIndex:       1,
		Values:           []int64{0xefef},
		AttributeIndices: []int32{1},
	}}
	otlpb.dictionary.AttributeTable = []*v1experimental.KeyValueAndUnit{{
		KeyStrindex: otlpb.addstr("process"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_StringValue{
				StringValue: "firefox",
			},
		},
	}, {
		KeyStrindex: otlpb.addstr("process"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_StringValue{
				StringValue: "chrome",
			},
		},
	}}
	otlpb.profile.SampleType = &v1experimental.ValueType{
		TypeStrindex: otlpb.addstr("samples"),
		UnitStrindex: otlpb.addstr("ms"),
	}
	otlpb.profile.TimeUnixNano = 239
	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{
					&otlpb.profile,
				}}}}},
		Dictionary: &otlpb.dictionary}
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, false)
	_, err := h.Export(context.Background(), req)
	assert.NoError(t, err)
	require.Equal(t, 1, len(profiles))
	require.Equal(t, 1, len(profiles[0].Series))

	seriesLabelsMap := make(map[string]string)
	for _, label := range profiles[0].Series[0].Labels {
		seriesLabelsMap[label.Name] = label.Value
	}
	assert.Equal(t, "", seriesLabelsMap["process"])
	assert.NotContains(t, seriesLabelsMap, "service.name")

	gp := profiles[0].Series[0].Profile.Profile

	jsonStr, err := strprofile.Stringify(gp, strprofile.Options{})
	assert.NoError(t, err)
	expectedJSON := readJSONFile(t, "testdata/TestSampleAttributes.json")
	assert.Equal(t, expectedJSON, jsonStr)

}

func TestDifferentServiceNames(t *testing.T) {
	// Create a profile with two samples having different service.name attributes
	// Expect them to be pushed as separate profiles
	svc := mockotlp.NewMockPushService(t)
	var profiles []*model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		c := (args.Get(1)).(*model.PushRequest)
		for _, series := range c.Series {
			sort.Sort(phlaremodel.Labels(series.Labels))
		}
		profiles = append(profiles, c)
	}).Return(nil, nil)

	otlpb := new(otlpbuilder)
	otlpb.dictionary.MappingTable = []*v1experimental.Mapping{{
		MemoryStart:      0x1000,
		MemoryLimit:      0x2000,
		FilenameStrindex: otlpb.addstr("service-a.so"),
	}, {
		MemoryStart:      0x2000,
		MemoryLimit:      0x3000,
		FilenameStrindex: otlpb.addstr("service-b.so"),
	}, {
		MemoryStart:      0x4000,
		MemoryLimit:      0x5000,
		FilenameStrindex: otlpb.addstr("service-c.so"),
	}}

	otlpb.dictionary.LocationTable = []*v1experimental.Location{{
		MappingIndex: 0, // service-a.so
		Address:      0x1100,
		Line: []*v1experimental.Line{{
			FunctionIndex: 0,
			Line:          10,
		}},
	}, {
		MappingIndex: 0, // service-a.so
		Address:      0x1200,
		Line: []*v1experimental.Line{{
			FunctionIndex: 1,
			Line:          20,
		}},
	}, {
		MappingIndex: 1, // service-b.so
		Address:      0x2100,
		Line: []*v1experimental.Line{{
			FunctionIndex: 2,
			Line:          30,
		}},
	}, {
		MappingIndex: 1, // service-b.so
		Address:      0x2200,
		Line: []*v1experimental.Line{{
			FunctionIndex: 3,
			Line:          40,
		}},
	}, {
		MappingIndex: 2, // service-c.so
		Address:      0xef0,
		Line: []*v1experimental.Line{{
			FunctionIndex: 4,
			Line:          50,
		}},
	}}

	otlpb.dictionary.FunctionTable = []*v1experimental.Function{{
		NameStrindex:       otlpb.addstr("serviceA_func1"),
		SystemNameStrindex: otlpb.addstr("serviceA_func1"),
		FilenameStrindex:   otlpb.addstr("service_a.go"),
	}, {
		NameStrindex:       otlpb.addstr("serviceA_func2"),
		SystemNameStrindex: otlpb.addstr("serviceA_func2"),
		FilenameStrindex:   otlpb.addstr("service_a.go"),
	}, {
		NameStrindex:       otlpb.addstr("serviceB_func1"),
		SystemNameStrindex: otlpb.addstr("serviceB_func1"),
		FilenameStrindex:   otlpb.addstr("service_b.go"),
	}, {
		NameStrindex:       otlpb.addstr("serviceB_func2"),
		SystemNameStrindex: otlpb.addstr("serviceB_func2"),
		FilenameStrindex:   otlpb.addstr("service_b.go"),
	}, {
		NameStrindex:       otlpb.addstr("serviceC_func3"),
		SystemNameStrindex: otlpb.addstr("serviceC_func3"),
		FilenameStrindex:   otlpb.addstr("service_c.go"),
	}}

	otlpb.dictionary.StackTable = []*v1experimental.Stack{{
		LocationIndices: []int32{0, 1}, // Use first two locations
	}, {
		LocationIndices: []int32{2, 3},
	}, {
		LocationIndices: []int32{4, 4},
	}}

	otlpb.profile.Sample = []*v1experimental.Sample{{
		StackIndex:       0,
		Values:           []int64{100},
		AttributeIndices: []int32{0},
	}, {
		StackIndex:       1,
		Values:           []int64{200},
		AttributeIndices: []int32{1},
	}, {
		StackIndex:       2,
		Values:           []int64{700},
		AttributeIndices: []int32{},
	}}

	otlpb.dictionary.AttributeTable = []*v1experimental.KeyValueAndUnit{{
		KeyStrindex: otlpb.addstr("service.name"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_StringValue{
				StringValue: "service-a",
			},
		},
	}, {
		KeyStrindex: otlpb.addstr("service.name"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_StringValue{
				StringValue: "service-b",
			},
		},
	}}

	otlpb.profile.SampleType = &v1experimental.ValueType{
		TypeStrindex: otlpb.addstr("samples"),
		UnitStrindex: otlpb.addstr("count"),
	}
	otlpb.profile.PeriodType = &v1experimental.ValueType{
		TypeStrindex: otlpb.addstr("cpu"),
		UnitStrindex: otlpb.addstr("nanoseconds"),
	}
	otlpb.profile.Period = 10000000 // 10ms
	otlpb.profile.TimeUnixNano = 239
	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{
					&otlpb.profile,
				}}}}},
		Dictionary: &otlpb.dictionary}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, false)
	_, err := h.Export(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(profiles))
	require.Equal(t, 3, len(profiles[0].Series))

	expectedProfiles := map[string]string{
		"{__delta__=\"false\", __name__=\"process_cpu\", __otel__=\"true\", service_name=\"service-a\"}":       "testdata/TestDifferentServiceNames_service_a_profile.json",
		"{__delta__=\"false\", __name__=\"process_cpu\", __otel__=\"true\", service_name=\"service-b\"}":       "testdata/TestDifferentServiceNames_service_b_profile.json",
		"{__delta__=\"false\", __name__=\"process_cpu\", __otel__=\"true\", service_name=\"unknown_service\"}": "testdata/TestDifferentServiceNames_unknown_profile.json",
	}

	for _, s := range profiles[0].Series {
		series := phlaremodel.Labels(s.Labels).ToPrometheusLabels().String()
		assert.Contains(t, expectedProfiles, series)
		expectedJsonPath := expectedProfiles[series]
		expectedJson := readJSONFile(t, expectedJsonPath)

		gp := s.Profile.Profile

		require.Equal(t, 1, len(gp.SampleType))
		assert.Equal(t, "cpu", gp.StringTable[gp.SampleType[0].Type])
		assert.Equal(t, "nanoseconds", gp.StringTable[gp.SampleType[0].Unit])

		require.NotNil(t, gp.PeriodType)
		assert.Equal(t, "cpu", gp.StringTable[gp.PeriodType.Type])
		assert.Equal(t, "nanoseconds", gp.StringTable[gp.PeriodType.Unit])
		assert.Equal(t, int64(10000000), gp.Period)

		jsonStr, err := strprofile.Stringify(gp, strprofile.Options{})
		assert.NoError(t, err)
		assert.JSONEq(t, expectedJson, jsonStr)
		assert.NotContains(t, jsonStr, "service.name")

	}
}

type otlpbuilder struct {
	profile    v1experimental.Profile
	dictionary v1experimental.ProfilesDictionary
	stringmap  map[string]int32
}

func (o *otlpbuilder) addstr(s string) int32 {
	if o.stringmap == nil {
		o.stringmap = make(map[string]int32)
	}
	if idx, ok := o.stringmap[s]; ok {
		return idx
	}
	idx := int32(len(o.stringmap))
	o.stringmap[s] = idx
	o.dictionary.StringTable = append(o.dictionary.StringTable, s)
	return idx
}

func testConfig() server.Config {
	cfg := server.Config{}
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	cfg.RegisterFlags(fs)
	return cfg
}
