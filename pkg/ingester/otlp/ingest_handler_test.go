package otlp

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/user"
	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	v1experimental2 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	v1 "go.opentelemetry.io/proto/otlp/common/v1"
	v1experimental "go.opentelemetry.io/proto/otlp/profiles/v1development"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/og/convert/pprof/strprofile"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockotlp"
	httputil "github.com/grafana/pyroscope/v2/pkg/util/http"
	"github.com/grafana/pyroscope/v2/pkg/validation"
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
			name: "use executable name",
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
					Lines:        nil,
				}, {
					MappingIndex: 0,
					Address:      0x2f0,
					Lines:        nil,
				}}
				b.dictionary.StackTable = []*v1experimental.Stack{{
					LocationIndices: []int32{0, 1},
				}}
				b.profile.SampleType = &v1experimental.ValueType{
					TypeStrindex: b.addstr("samples"),
					UnitStrindex: b.addstr("ms"),
				}
				b.profile.Samples = []*v1experimental.Sample{{
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
					TypeStrindex: b.addstr("off_cpu"),
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
				b.profile.Samples = []*v1experimental.Sample{{
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
					TypeStrindex: b.addstr("period_type"),
					UnitStrindex: b.addstr("period_unit"),
				}
				b.profile.Period = 100
				b.profile.Samples = []*v1experimental.Sample{{
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
			h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
			_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)

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
	otlpb.profile.Samples = []*v1experimental.Sample{{
		StackIndex:       0,
		Values:           []int64{0xef},
		AttributeIndices: []int32{0, 2},
	}, {
		StackIndex:       1,
		Values:           []int64{0xefef},
		AttributeIndices: []int32{1, 3},
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
	}, {
		KeyStrindex: otlpb.addstr("cpu.logical_number"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_IntValue{
				IntValue: 0,
			},
		},
	}, {
		KeyStrindex: otlpb.addstr("cpu.logical_number"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_IntValue{
				IntValue: 7,
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
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
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

func TestSampleAttributesWithSliceValues(t *testing.T) {
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
	otlpb.profile.Samples = []*v1experimental.Sample{{
		StackIndex:       0,
		Values:           []int64{0xef},
		AttributeIndices: []int32{0, 2},
	}, {
		StackIndex:       1,
		Values:           []int64{0xefef},
		AttributeIndices: []int32{1, 3},
	}}
	otlpb.dictionary.AttributeTable = []*v1experimental.KeyValueAndUnit{{
		KeyStrindex: otlpb.addstr("process"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_ArrayValue{
				ArrayValue: &v1.ArrayValue{
					Values: []*v1.AnyValue{{
						Value: &v1.AnyValue_StringValue{
							StringValue: "firefox",
						},
					}},
				},
			},
		},
	}, {
		KeyStrindex: otlpb.addstr("process"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_ArrayValue{
				ArrayValue: &v1.ArrayValue{
					Values: []*v1.AnyValue{{
						Value: &v1.AnyValue_StringValue{
							StringValue: "chrome",
						},
					}},
				},
			},
		},
	}, {
		KeyStrindex: otlpb.addstr("cpu.logical_number"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_ArrayValue{
				ArrayValue: &v1.ArrayValue{
					Values: []*v1.AnyValue{{
						Value: &v1.AnyValue_IntValue{
							IntValue: 0,
						},
					}},
				},
			},
		},
	}, {
		KeyStrindex: otlpb.addstr("cpu.logical_number"),
		Value: &v1.AnyValue{
			Value: &v1.AnyValue_ArrayValue{
				ArrayValue: &v1.ArrayValue{
					Values: []*v1.AnyValue{{
						Value: &v1.AnyValue_IntValue{
							IntValue: 7,
						},
					}},
				},
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
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
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

func TestStringValueFromAnyValue(t *testing.T) {
	tests := []struct {
		name     string
		value    *v1.AnyValue
		expected string
	}{
		{
			name:     "nil value",
			value:    nil,
			expected: "",
		},
		{
			name: "scalar string",
			value: &v1.AnyValue{
				Value: &v1.AnyValue_StringValue{StringValue: "hello"},
			},
			expected: "hello",
		},
		{
			name: "single-element string slice",
			value: &v1.AnyValue{
				Value: &v1.AnyValue_ArrayValue{
					ArrayValue: &v1.ArrayValue{
						Values: []*v1.AnyValue{{
							Value: &v1.AnyValue_StringValue{StringValue: "hello"},
						}},
					},
				},
			},
			expected: "hello",
		},
		{
			name: "multi-element slice returns empty",
			value: &v1.AnyValue{
				Value: &v1.AnyValue_ArrayValue{
					ArrayValue: &v1.ArrayValue{
						Values: []*v1.AnyValue{
							{Value: &v1.AnyValue_StringValue{StringValue: "a"}},
							{Value: &v1.AnyValue_StringValue{StringValue: "b"}},
						},
					},
				},
			},
			expected: "",
		},
		{
			name: "empty slice returns empty",
			value: &v1.AnyValue{
				Value: &v1.AnyValue_ArrayValue{
					ArrayValue: &v1.ArrayValue{
						Values: []*v1.AnyValue{},
					},
				},
			},
			expected: "",
		},
		{
			name: "int value returns string",
			value: &v1.AnyValue{
				Value: &v1.AnyValue_IntValue{IntValue: 42},
			},
			expected: "42",
		},
		{
			name: "zero int value returns string zero",
			value: &v1.AnyValue{
				Value: &v1.AnyValue_IntValue{IntValue: 0},
			},
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringValueFromAnyValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendAttributesUniqueWithSliceValues(t *testing.T) {
	existingAttrs := []*typesv1.LabelPair{
		{Name: "existing", Value: "value"},
	}
	newAttrs := []*v1.KeyValue{
		{
			Key: "slice_attr",
			Value: &v1.AnyValue{
				Value: &v1.AnyValue_ArrayValue{
					ArrayValue: &v1.ArrayValue{
						Values: []*v1.AnyValue{{
							Value: &v1.AnyValue_StringValue{
								StringValue: "unwrapped",
							},
						}},
					},
				},
			},
		},
	}
	processedKeys := map[string]bool{"existing": true}
	result := appendAttributesUnique(existingAttrs, newAttrs, processedKeys)
	assert.Equal(t, []*typesv1.LabelPair{
		{Name: "existing", Value: "value"},
		{Name: "slice_attr", Value: "unwrapped"},
	}, result)
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
		Lines: []*v1experimental.Line{{
			FunctionIndex: 0,
			Line:          10,
		}},
	}, {
		MappingIndex: 0, // service-a.so
		Address:      0x1200,
		Lines: []*v1experimental.Line{{
			FunctionIndex: 1,
			Line:          20,
		}},
	}, {
		MappingIndex: 1, // service-b.so
		Address:      0x2100,
		Lines: []*v1experimental.Line{{
			FunctionIndex: 2,
			Line:          30,
		}},
	}, {
		MappingIndex: 1, // service-b.so
		Address:      0x2200,
		Lines: []*v1experimental.Line{{
			FunctionIndex: 3,
			Line:          40,
		}},
	}, {
		MappingIndex: 2, // service-c.so
		Address:      0xef0,
		Lines: []*v1experimental.Line{{
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

	otlpb.profile.Samples = []*v1experimental.Sample{{
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
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
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

func defaultLimits() validation.MockLimits {
	return validation.MockLimits{
		IngestionBodyLimitBytesValue: 1024 * 1024 * 1024, // 1GB
	}
}

// createValidOTLPRequest creates a minimal valid OTLP profile export request for testing
func createValidOTLPRequest() *v1experimental2.ExportProfilesServiceRequest {
	b := new(otlpbuilder)
	b.dictionary.MappingTable = []*v1experimental.Mapping{{
		MemoryStart:      0x1000,
		MemoryLimit:      0x2000,
		FilenameStrindex: b.addstr("test.so"),
	}}
	b.dictionary.LocationTable = []*v1experimental.Location{{
		MappingIndex: 0,
		Address:      0x1100,
	}}
	b.dictionary.StackTable = []*v1experimental.Stack{{
		LocationIndices: []int32{0},
	}}
	b.profile.SampleType = &v1experimental.ValueType{
		TypeStrindex: b.addstr("samples"),
		UnitStrindex: b.addstr("count"),
	}
	b.profile.Samples = []*v1experimental.Sample{{
		StackIndex: 0,
		Values:     []int64{100},
	}}
	b.profile.TimeUnixNano = 1234567890

	return &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{&b.profile},
			}},
		}},
		Dictionary: &b.dictionary,
	}
}

func TestHTTPRequestWithJSONAndTenantAccepted(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

	jsonRequest := `{
		"resourceProfiles": [{
			"scopeProfiles": [{
				"profiles": [{
					"sampleType": {"typeStrindex": 0, "unitStrindex": 1},
					"samples": [{"stackIndex": 0, "values": [100]}],
					"timeUnixNano": "1234567890"
				}]
			}]
		}],
		"dictionary": {
			"stringTable": ["samples", "count", "test.so"],
			"mappingTable": [{"memoryStart": "4096", "memoryLimit": "8192", "filenameStrindex": 2}],
			"locationTable": [{"mappingIndex": 0, "address": "4352"}],
			"stackTable": [{"locationIndices": [0]}]
		}
	}`

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader([]byte(jsonRequest)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(user.OrgIDHeaderName, "json-tenant")

	w := httptest.NewRecorder()
	httputil.AuthenticateUser(true).Wrap(h).ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "json-tenant", capturedTenantID)
}

func TestExportSetsDecompressedSizeForOTLP(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var capturedReq *model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		capturedReq = args.Get(1).(*model.PushRequest)
	}).Return(nil)

	req := createValidOTLPRequest()
	expectedSize := proto.Size(req.ResourceProfiles[0].ScopeProfiles[0].Profiles[0])

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)
	require.NotNil(t, capturedReq)
	assert.Equal(t, expectedSize, capturedReq.ReceivedDecompressedProfileSize)
}

func TestHTTPRequestWithGzipCompression(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

	req := createValidOTLPRequest()
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	_, err = gzipWriter.Write(reqBytes)
	require.NoError(t, err)
	err = gzipWriter.Close()
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(gzipBuf.Bytes()))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "gzip")

	w := httptest.NewRecorder()
	httputil.AuthenticateUser(false).Wrap(h).ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, tenant.DefaultTenantID, capturedTenantID)
}

func TestHTTPRequestWithGzipCompressionAndJSON(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

	jsonRequest := `{
		"resourceProfiles": [{
			"scopeProfiles": [{
				"profiles": [{
					"sampleType": {"typeStrindex": 0, "unitStrindex": 1},
					"samples": [{"stackIndex": 0, "values": [100]}],
					"timeUnixNano": "1234567890"
				}]
			}]
		}],
		"dictionary": {
			"stringTable": ["samples", "count", "test.so"],
			"mappingTable": [{"memoryStart": "4096", "memoryLimit": "8192", "filenameStrindex": 2}],
			"locationTable": [{"mappingIndex": 0, "address": "4352"}],
			"stackTable": [{"locationIndices": [0]}]
		}
	}`

	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	_, err := gzipWriter.Write([]byte(jsonRequest))
	require.NoError(t, err)
	err = gzipWriter.Close()
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(gzipBuf.Bytes()))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Content-Encoding", "gzip")
	httpReq.Header.Set(user.OrgIDHeaderName, "gzip-json-tenant")

	w := httptest.NewRecorder()
	httputil.AuthenticateUser(true).Wrap(h).ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip-json-tenant", capturedTenantID)
}

// recordingPushService captures every PushRequest passed to PushBatch and can be
// configured to return an error.
func recordPushBatch(t *testing.T) (*mockotlp.MockPushService, *[]*model.PushRequest) {
	t.Helper()
	svc := mockotlp.NewMockPushService(t)
	var profiles []*model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		c := (args.Get(1)).(*model.PushRequest)
		for _, series := range c.Series {
			sort.Sort(phlaremodel.Labels(series.Labels))
		}
		profiles = append(profiles, c)
	}).Return(nil)
	return svc, &profiles
}

// newCPUProfile builds a minimal profile that references the shared builder's
// dictionary. value is the single sample value; the SampleType/PeriodType are
// keyed by the provided unit strings so callers can force compatible/incompatible
// merges.
func newCPUProfile(b *otlpbuilder, sampleUnit, periodUnit string, value, timeUnixNano, durationNanos uint64) *v1experimental.Profile {
	stackIdx := int32(len(b.dictionary.StackTable))
	locIdx := int32(len(b.dictionary.LocationTable))
	mapIdx := int32(len(b.dictionary.MappingTable))
	funcIdx := int32(len(b.dictionary.FunctionTable))

	b.dictionary.FunctionTable = append(b.dictionary.FunctionTable, &v1experimental.Function{
		NameStrindex:       b.addstr("shared_func"),
		SystemNameStrindex: b.addstr("shared_func"),
		FilenameStrindex:   b.addstr("shared.go"),
	})
	b.dictionary.MappingTable = append(b.dictionary.MappingTable, &v1experimental.Mapping{
		MemoryStart:      0x1000,
		MemoryLimit:      0x2000,
		FilenameStrindex: b.addstr("shared.so"),
	})
	b.dictionary.LocationTable = append(b.dictionary.LocationTable, &v1experimental.Location{
		MappingIndex: mapIdx,
		Address:      0x1100,
		Lines: []*v1experimental.Line{{
			FunctionIndex: funcIdx,
			Line:          10,
		}},
	})
	b.dictionary.StackTable = append(b.dictionary.StackTable, &v1experimental.Stack{
		LocationIndices: []int32{locIdx},
	})

	return &v1experimental.Profile{
		SampleType: &v1experimental.ValueType{
			TypeStrindex: b.addstr("samples"),
			UnitStrindex: b.addstr(sampleUnit),
		},
		PeriodType: &v1experimental.ValueType{
			TypeStrindex: b.addstr("cpu"),
			UnitStrindex: b.addstr(periodUnit),
		},
		Period:       10000000,
		TimeUnixNano: timeUnixNano,
		DurationNano: durationNanos,
		Samples: []*v1experimental.Sample{{
			StackIndex: stackIdx,
			Values:     []int64{int64(value)},
		}},
	}
}

// reproducing test: two identical Profile messages in a single
// ScopeProfiles must produce a single PushBatch call with a single merged series.
func TestExport_BatchesAcrossProfileMessages(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, "count", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "count", "nanoseconds", 7, 200, 20)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once")
	require.Equal(t, 1, len((*profiles)[0].Series), "identical series must merge into one")
	assert.Equal(t, model.RawProfileTypeOTEL, (*profiles)[0].RawProfileType)
}

// merging the same series sums sample values and combines headers
// (smallest TimeNanos, summed DurationNanos).
func TestExport_MergesSameSeriesValues(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, "count", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "count", "nanoseconds", 7, 50, 20)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles))
	require.Equal(t, 1, len((*profiles)[0].Series))

	gp := (*profiles)[0].Series[0].Profile.Profile
	require.Equal(t, 1, len(gp.Sample), "the shared stack must collapse to one sample")
	var total int64
	for _, v := range gp.Sample[0].Value {
		total += v
	}
	// The "samples:count:cpu:nanoseconds" conversion scales each value by the
	// period (10ms here); the merge then sums them: (5+7)*10000000.
	assert.Equal(t, int64(12*10000000), total, "merged sample value must be the sum of inputs")
	assert.Equal(t, int64(50), gp.TimeNanos, "TimeNanos must be the smallest")
	assert.Equal(t, int64(30), gp.DurationNanos, "DurationNanos must be summed")
}

// Received{Compressed,Decompressed}ProfileSize must be the sum of
// proto.Size over every Profile message.
func TestExport_SumsReceivedSizesAcrossMessages(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var capturedReq *model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		capturedReq = args.Get(1).(*model.PushRequest)
	}).Return(nil)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, "count", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "count", "nanoseconds", 7, 200, 20)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	expectedSize := proto.Size(p0) + proto.Size(p1)

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)
	require.NotNil(t, capturedReq)
	assert.Equal(t, expectedSize, capturedReq.ReceivedDecompressedProfileSize)
	assert.Equal(t, expectedSize, capturedReq.ReceivedCompressedProfileSize)
}

// distinct services across messages stay separate but collapse into a
// single PushBatch call.
func TestExport_DistinctServicesAcrossMessages(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, "count", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "count", "nanoseconds", 7, 200, 20)

	// Give each profile a per-sample service.name attribute so they key to
	// distinct services in conversion.
	attrA := int32(len(b.dictionary.AttributeTable))
	b.dictionary.AttributeTable = append(b.dictionary.AttributeTable, &v1experimental.KeyValueAndUnit{
		KeyStrindex: b.addstr("service.name"),
		Value:       &v1.AnyValue{Value: &v1.AnyValue_StringValue{StringValue: "service-a"}},
	})
	attrB := int32(len(b.dictionary.AttributeTable))
	b.dictionary.AttributeTable = append(b.dictionary.AttributeTable, &v1experimental.KeyValueAndUnit{
		KeyStrindex: b.addstr("service.name"),
		Value:       &v1.AnyValue{Value: &v1.AnyValue_StringValue{StringValue: "service-b"}},
	})
	p0.Samples[0].AttributeIndices = []int32{attrA}
	p1.Samples[0].AttributeIndices = []int32{attrB}

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{
				{Profiles: []*v1experimental.Profile{p0}},
				{Profiles: []*v1experimental.Profile{p1}},
			},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once")
	require.Equal(t, 2, len((*profiles)[0].Series), "distinct services must not merge")

	services := map[string]bool{}
	for _, s := range (*profiles)[0].Series {
		for _, l := range s.Labels {
			if l.Name == phlaremodel.LabelNameServiceName {
				services[l.Value] = true
			}
		}
	}
	assert.Equal(t, map[string]bool{"service-a": true, "service-b": true}, services)
}

// identical label sets across distinct ScopeProfiles merge into one series.
func TestExport_MergesAcrossScopeProfiles(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, "count", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "count", "nanoseconds", 7, 200, 20)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{
				{Profiles: []*v1experimental.Profile{p0}},
				{Profiles: []*v1experimental.Profile{p1}},
			},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles))
	require.Equal(t, 1, len((*profiles)[0].Series))
}

// two messages with the same label set but incompatible headers
// (differing period unit) must each be emitted as their own series, with no
// error and a single PushBatch call.
func TestExport_IncompatibleSampleTypesFallBack(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	// Same sample type / profile-name label, but different period unit ->
	// combineHeaders' compatible() check fails on the second merge.
	p0 := newCPUProfile(b, "count", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "count", "milliseconds", 7, 200, 20)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err, "incompatible profiles must not fail the request")

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once")
	require.Equal(t, 2, len((*profiles)[0].Series), "incompatible profiles must be emitted separately")
}

// a known validation error from PushBatch maps to HTTP 400; an unknown
// error maps to HTTP 500. With a single PushBatch the error may be a multierror;
// isKnownValidationError inspects only the top-level error, so these tests pin
// the single-error cases. Mixed validation+internal errors are classified by the
// top-level wrapper (documented, not asserted).
func TestExport_ValidationErrorMapsTo400(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	svc.On("PushBatch", mock.Anything, mock.Anything).
		Return(validation.NewErrorf(validation.BodySizeLimit, "too big"))

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

	w := doValidOTLPHTTPRequest(t, h)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExport_InternalErrorMapsTo500(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	svc.On("PushBatch", mock.Anything, mock.Anything).
		Return(errors.New("boom"))

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

	w := doValidOTLPHTTPRequest(t, h)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// doValidOTLPHTTPRequest drives a valid protobuf OTLP request through the HTTP
// handler and returns the recorder.
func doValidOTLPHTTPRequest(t *testing.T, h Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := createValidOTLPRequest()
	body, err := proto.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set(user.OrgIDHeaderName, "test-tenant")

	w := httptest.NewRecorder()
	httputil.AuthenticateUser(true).Wrap(h).ServeHTTP(w, httpReq)
	return w
}

// newNilPeriodTypeProfile builds a minimal profile WITHOUT a PeriodType,
// matching the shape produced by createValidOTLPRequest() and the common
// OTLP/eBPF case. All such profiles share one stack/location so they merge into
// a single series.
func newNilPeriodTypeProfile(b *otlpbuilder, value, timeUnixNano uint64) *v1experimental.Profile {
	stackIdx := int32(len(b.dictionary.StackTable))
	locIdx := int32(len(b.dictionary.LocationTable))
	mapIdx := int32(len(b.dictionary.MappingTable))
	funcIdx := int32(len(b.dictionary.FunctionTable))

	b.dictionary.FunctionTable = append(b.dictionary.FunctionTable, &v1experimental.Function{
		NameStrindex:       b.addstr("nilpt_func"),
		SystemNameStrindex: b.addstr("nilpt_func"),
		FilenameStrindex:   b.addstr("nilpt.go"),
	})
	b.dictionary.MappingTable = append(b.dictionary.MappingTable, &v1experimental.Mapping{
		MemoryStart:      0x1000,
		MemoryLimit:      0x2000,
		FilenameStrindex: b.addstr("nilpt.so"),
	})
	b.dictionary.LocationTable = append(b.dictionary.LocationTable, &v1experimental.Location{
		MappingIndex: mapIdx,
		Address:      0x1100,
		Lines: []*v1experimental.Line{{
			FunctionIndex: funcIdx,
			Line:          10,
		}},
	})
	b.dictionary.StackTable = append(b.dictionary.StackTable, &v1experimental.Stack{
		LocationIndices: []int32{locIdx},
	})

	return &v1experimental.Profile{
		SampleType: &v1experimental.ValueType{
			TypeStrindex: b.addstr("samples"),
			UnitStrindex: b.addstr("count"),
		},
		// No PeriodType — the common OTLP/eBPF shape.
		TimeUnixNano: timeUnixNano,
		Samples: []*v1experimental.Sample{{
			StackIndex: stackIdx,
			Values:     []int64{int64(value)},
		}},
	}
}

// TestExport_ThreeIncompatibleSameLabelSet_NoHang guards CRITICAL #1: three
// Profile messages sharing one label set but pairwise-incompatible headers must
// not hang the export call (the old constant-XOR fallback-key search looped
// forever on the 3rd message). The call must complete and emit one PushBatch
// with three separate series. This calls Export synchronously; if an unbounded
// loop is reintroduced, the go test package timeout will hang the test binary
// and fail here.
func TestExport_ThreeIncompatibleSameLabelSet_NoHang(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	// Same profile-name label (the sample unit avoids the special
	// "samples:count:cpu:nanoseconds" case, so all three land on the custom
	// "cpu" profile name and thus share one label set), but three distinct
	// period units, so each fails to merge into the previous accumulator.
	p0 := newCPUProfile(b, "milliseconds", "nanoseconds", 5, 100, 10)
	p1 := newCPUProfile(b, "milliseconds", "milliseconds", 7, 200, 20)
	p2 := newCPUProfile(b, "milliseconds", "microseconds", 9, 300, 30)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1, p2},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err, "incompatible profiles must not fail the request")

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once")
	require.Equal(t, 3, len((*profiles)[0].Series), "three incompatible profiles must be emitted separately")
}

// TestExport_NilPeriodType_MergesToOneSeries guards CRITICAL #2 / HIGH #3: two
// Profile messages sharing a label set with a nil PeriodType (the common
// OTLP/eBPF shape, matching createValidOTLPRequest) must merge into a single
// series with summed values, in one PushBatch call. Before the equalValueType
// fix this produced two series.
func TestExport_NilPeriodType_MergesToOneSeries(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	p0 := newNilPeriodTypeProfile(b, 5, 100)
	p1 := newNilPeriodTypeProfile(b, 7, 200)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once")
	require.Equal(t, 1, len((*profiles)[0].Series), "nil-PeriodType profiles with the same label set must merge into one series")

	gp := (*profiles)[0].Series[0].Profile.Profile
	require.Equal(t, 1, len(gp.Sample), "the shared stack must collapse to one sample")
	var total int64
	for _, v := range gp.Sample[0].Value {
		total += v
	}
	assert.Equal(t, int64(12), total, "merged sample value must be the sum of inputs (5+7)")
}
