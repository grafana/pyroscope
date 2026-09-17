package otlp

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/user"
	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
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

const otlpProfileJSON = `{
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

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader([]byte(otlpProfileJSON)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(user.OrgIDHeaderName, "json-tenant")

	w := httptest.NewRecorder()
	httputil.AuthenticateUser(true).Wrap(h).ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "json-tenant", capturedTenantID)
}

func TestHTTPExportErrorStatusCodes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pushErr    error
		wantStatus int
	}{
		{
			name:       "tenant over ingestion limit is a client error (429)",
			pushErr:    connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("limit of 0 B/month reached, next reset at 2026-08-01T00:00:00Z")),
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "validation failure is a client error (400)",
			pushErr:    validation.NewErrorf(validation.ProfileSizeLimit, "profile size exceeds limit"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unexpected push failure is a server error (500)",
			pushErr:    fmt.Errorf("ingester unreachable"),
			wantStatus: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := mockotlp.NewMockPushService(t)
			svc.On("PushBatch", mock.Anything, mock.Anything).Return(tc.pushErr)

			logger := test.NewTestingLogger(t)
			h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

			httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader([]byte(otlpProfileJSON)))
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set(user.OrgIDHeaderName, "tenant-a")

			w := httptest.NewRecorder()
			httputil.AuthenticateUser(true).Wrap(h).ServeHTTP(w, httpReq)

			assert.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestExportGRPCStatusCodes(t *testing.T) {
	req := &v1experimental2.ExportProfilesServiceRequest{}
	require.NoError(t, protojson.Unmarshal([]byte(otlpProfileJSON), req))

	for _, tc := range []struct {
		name     string
		pushErr  error
		wantCode codes.Code
	}{
		{
			name:     "tenant over ingestion limit maps to ResourceExhausted",
			pushErr:  connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("limit of 0 B/month reached")),
			wantCode: codes.ResourceExhausted,
		},
		{
			name:     "unexpected push failure stays Unknown",
			pushErr:  fmt.Errorf("ingester unreachable"),
			wantCode: codes.Unknown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := mockotlp.NewMockPushService(t)
			svc.On("PushBatch", mock.Anything, mock.Anything).Return(tc.pushErr)

			logger := test.NewTestingLogger(t)
			h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())

			ctx := user.InjectOrgID(context.Background(), "tenant-a")
			_, err := h.Export(ctx, req)
			require.Error(t, err)
			assert.Equal(t, tc.wantCode, status.Code(err))
		})
	}
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

	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	_, err := gzipWriter.Write([]byte(otlpProfileJSON))
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

// TestExport_SinglePushBatchPerExport verifies that profiles spread across
// multiple ResourceProfiles/ScopeProfiles collapse into exactly one PushBatch
// call, with every converted profile accumulated into that single request.
func TestExport_SinglePushBatchPerExport(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, 5, 100, 10)
	p1 := newCPUProfile(b, 7, 200, 20)
	p2 := newCPUProfile(b, 9, 300, 30)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{
				{Profiles: []*v1experimental.Profile{p0}},
				{Profiles: []*v1experimental.Profile{p1}},
			},
		}, {
			ScopeProfiles: []*v1experimental.ScopeProfiles{
				{Profiles: []*v1experimental.Profile{p2}},
			},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once per export")
	require.Equal(t, 3, len((*profiles)[0].Series), "every converted profile must accumulate into the single request")
	assert.Equal(t, model.RawProfileTypeOTEL, (*profiles)[0].RawProfileType)
}

// TestExport_SeriesCountEqualsConvertedProfiles verifies that N messages with
// distinct services produce N separate series in a single PushBatch call.
func TestExport_SeriesCountEqualsConvertedProfiles(t *testing.T) {
	svc, profiles := recordPushBatch(t)

	services := []string{"svc-a", "svc-b", "svc-c"}

	b := new(otlpbuilder)
	sps := make([]*v1experimental.ScopeProfiles, 0, len(services))
	for i, name := range services {
		p := newCPUProfile(b, uint64(5+i), uint64(100+i*10), 10)
		attrIdx := int32(len(b.dictionary.AttributeTable))
		b.dictionary.AttributeTable = append(b.dictionary.AttributeTable, &v1experimental.KeyValueAndUnit{
			KeyStrindex: b.addstr("service.name"),
			Value:       &v1.AnyValue{Value: &v1.AnyValue_StringValue{StringValue: name}},
		})
		p.Samples[0].AttributeIndices = []int32{attrIdx}
		sps = append(sps, &v1experimental.ScopeProfiles{Profiles: []*v1experimental.Profile{p}})
	}

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{ScopeProfiles: sps}},
		Dictionary:       &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)

	require.Equal(t, 1, len(*profiles), "PushBatch must be called exactly once")
	require.Equal(t, len(services), len((*profiles)[0].Series), "series count must equal the number of converted profiles")

	got := map[string]bool{}
	for _, s := range (*profiles)[0].Series {
		for _, l := range s.Labels {
			if l.Name == phlaremodel.LabelNameServiceName {
				got[l.Value] = true
			}
		}
	}
	assert.Equal(t, map[string]bool{"svc-a": true, "svc-b": true, "svc-c": true}, got)
}

// TestExport_SumsReceivedSizesAcrossMessages verifies that both
// Received{Compressed,Decompressed}ProfileSize equal the sum of proto.Size over
// every Profile message, measured before conversion (ConvertOtelToGoogle mutates
// the profile in place, so measuring after would misreport the size).
func TestExport_SumsReceivedSizesAcrossMessages(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var capturedReq *model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		capturedReq = args.Get(1).(*model.PushRequest)
	}).Return(nil)

	b := new(otlpbuilder)
	p0 := newCPUProfile(b, 5, 100, 10)
	p1 := newCPUProfile(b, 7, 200, 20)

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p0, p1},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	// Measured before Export: ConvertOtelToGoogle mutates the profiles in place.
	expectedSize := proto.Size(p0) + proto.Size(p1)

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)
	require.NotNil(t, capturedReq)
	assert.Equal(t, expectedSize, capturedReq.ReceivedDecompressedProfileSize)
	assert.Equal(t, expectedSize, capturedReq.ReceivedCompressedProfileSize)
}

// TestExport_EmptyRequest_NoPushBatch_Success verifies that a request whose
// profiles yield zero series does NOT call PushBatch and returns success (a
// zero-series PushBatch would otherwise be a spurious error).
func TestExport_EmptyRequest_NoPushBatch_Success(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	var profiles []*model.PushRequest
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		profiles = append(profiles, args.Get(1).(*model.PushRequest))
	}).Return(nil).Maybe()

	b := new(otlpbuilder)
	// A profile with no samples converts to zero series.
	p := &v1experimental.Profile{TimeUnixNano: 239}

	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{p},
			}},
		}},
		Dictionary: &b.dictionary,
	}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.NoError(t, err)
	require.Equal(t, 0, len(profiles), "PushBatch must not be called when there are no series")
}

// TestExport_PushBatchError_Propagates verifies that an error from PushBatch is
// propagated (wrapped) back out of Export.
func TestExport_PushBatchError_Propagates(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	svc.On("PushBatch", mock.Anything, mock.Anything).Return(assert.AnError)

	req := createValidOTLPRequest()

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to make a GRPC request")
}

// recordPushBatch wires a mock PushService that records every PushRequest it
// receives (labels sorted for deterministic comparison).
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
// fixed to "samples"/"count" and "cpu"/"nanoseconds" so repeated calls share a
// label set.
func newCPUProfile(b *otlpbuilder, value, timeUnixNano, durationNanos uint64) *v1experimental.Profile {
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
			UnitStrindex: b.addstr("count"),
		},
		PeriodType: &v1experimental.ValueType{
			TypeStrindex: b.addstr("cpu"),
			UnitStrindex: b.addstr("nanoseconds"),
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
