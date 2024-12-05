package otlp

import (
	"connectrpc.com/connect"
	"context"
	"fmt"
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	v1experimental2 "github.com/grafana/pyroscope/api/otlp/collector/profiles/v1experimental"
	"github.com/grafana/pyroscope/api/otlp/profiles/v1experimental"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockotlp"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	v1 "github.com/grafana/pyroscope/api/otlp/common/v1"
)

func TestGetServiceNameFromAttributes(t *testing.T) {
	tests := []struct {
		name     string
		attrs    []v1.KeyValue
		expected string
	}{
		{
			name:     "empty attributes",
			attrs:    []v1.KeyValue{},
			expected: "unknown",
		},
		{
			name: "service name present",
			attrs: []v1.KeyValue{
				{
					Key: "service.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "test-service",
						},
					},
				},
			},
			expected: "test-service",
		},
		{
			name: "service name empty",
			attrs: []v1.KeyValue{
				{
					Key: "service.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "",
						},
					},
				},
			},
			expected: "unknown",
		},
		{
			name: "service name among other attributes",
			attrs: []v1.KeyValue{
				{
					Key: "host.name",
					Value: v1.AnyValue{
						Value: &v1.AnyValue_StringValue{
							StringValue: "host1",
						},
					},
				},
				{
					Key: "service.name",
					Value: v1.AnyValue{
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
		newAttrs      []v1.KeyValue
		processedKeys map[string]bool
		expected      []*typesv1.LabelPair
	}{
		{
			name:          "empty attributes",
			existingAttrs: []*typesv1.LabelPair{},
			newAttrs:      []v1.KeyValue{},
			processedKeys: make(map[string]bool),
			expected:      []*typesv1.LabelPair{},
		},
		{
			name: "new unique attributes",
			existingAttrs: []*typesv1.LabelPair{
				{Name: "existing", Value: "value"},
			},
			newAttrs: []v1.KeyValue{
				{
					Key: "new",
					Value: v1.AnyValue{
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
			newAttrs: []v1.KeyValue{
				{
					Key: "key1",
					Value: v1.AnyValue{
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

func TestSymbolizedFunctionNames(t *testing.T) {
	// Create two unsymbolized locations at 0x1e0 and 0x2f0
	// Expect both of them to be present in the converted pprof
	svc := mockotlp.NewMockPushService(t)
	var profiles [][]byte
	svc.On("Push", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		c := (args.Get(1)).(*connect.Request[pushv1.PushRequest])
		profiles = append(profiles, c.Msg.Series[0].Samples[0].RawProfile)
	}).Return(nil, nil)

	otlpb := new(otlpbuilder)
	otlpb.profile.Mapping = []*v1experimental.Mapping{{
		MemoryStart: 0x1000,
		MemoryLimit: 0x1000,
		Filename:    otlpb.addstr("file1.so"),
	}}
	otlpb.profile.Location = []*v1experimental.Location{{
		MappingIndex: 0,
		Address:      0x1e0,
		Line:         nil,
	}, {
		MappingIndex: 0,
		Address:      0x2f0,
		Line:         nil,
	}}
	otlpb.profile.LocationIndices = []int64{0, 1}
	otlpb.profile.Sample = []*v1experimental.Sample{{
		LocationsStartIndex: 0,
		LocationsLength:     2,
		Value:               []int64{0xef},
	}}
	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.ProfileContainer{{
					Profile: &otlpb.profile,
				}}}}}}}
	logger := testutil.NewLogger(t)
	h := NewOTLPIngestHandler(svc, logger, false)
	_, err := h.Export(context.Background(), req)
	assert.NoError(t, err)
	require.Equal(t, 1, len(profiles))

	gp := new(googlev1.Profile)
	err = gp.UnmarshalVT(profiles[0])
	require.NoError(t, err)

	ss := bench.StackCollapseProtoWithOptions(gp, bench.StackCollapseOptions{
		ValueIdx:   0,
		Scale:      1,
		WithLabels: true,
	})
	require.Equal(t, 1, len(ss))
	require.Equal(t, " ||| file1.so 0x2f0;file1.so 0x1e0 239", ss[0])
}

func TestSampleAttributes(t *testing.T) {
	// Create a profile with two samples, with different sample attributes
	// one process=firefox, the other process=chrome
	// expect both of them to be present in the converted pprof as labels, but not series labels
	svc := mockotlp.NewMockPushService(t)
	var profiles []*pushv1.PushRequest
	svc.On("Push", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		c := (args.Get(1)).(*connect.Request[pushv1.PushRequest])
		profiles = append(profiles, c.Msg)
	}).Return(nil, nil)

	otlpb := new(otlpbuilder)
	otlpb.profile.Mapping = []*v1experimental.Mapping{{
		MemoryStart: 0x1000,
		MemoryLimit: 0x1000,
		Filename:    otlpb.addstr("firefox.so"),
	}, {
		MemoryStart: 0x1000,
		MemoryLimit: 0x1000,
		Filename:    otlpb.addstr("chrome.so"),
	}}

	otlpb.profile.Location = []*v1experimental.Location{{
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
	otlpb.profile.LocationIndices = []int64{0, 1, 2, 3}
	otlpb.profile.Sample = []*v1experimental.Sample{{
		LocationsStartIndex: 0,
		LocationsLength:     2,
		Value:               []int64{0xef},
		Attributes:          []uint64{0},
	}, {
		LocationsStartIndex: 2,
		LocationsLength:     2,
		Value:               []int64{0xefef},
		Attributes:          []uint64{1},
	}}
	otlpb.profile.AttributeTable = []v1.KeyValue{{
		Key: "process",
		Value: v1.AnyValue{
			Value: &v1.AnyValue_StringValue{
				StringValue: "firefox",
			},
		},
	}, {
		Key: "process",
		Value: v1.AnyValue{
			Value: &v1.AnyValue_StringValue{
				StringValue: "chrome",
			},
		},
	}}
	req := &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.ProfileContainer{{
					Profile: &otlpb.profile,
				}}}}}}}
	logger := testutil.NewLogger(t)
	h := NewOTLPIngestHandler(svc, logger, false)
	_, err := h.Export(context.Background(), req)
	assert.NoError(t, err)
	require.Equal(t, 1, len(profiles))
	require.Equal(t, 1, len(profiles[0].Series))
	require.Equal(t, 1, len(profiles[0].Series[0].Samples))

	seriesLabelsMap := make(map[string]string)
	for _, label := range profiles[0].Series[0].Labels {
		seriesLabelsMap[label.Name] = label.Value
	}
	assert.Equal(t, "", seriesLabelsMap["process"])

	gp := new(googlev1.Profile)
	err = gp.UnmarshalVT(profiles[0].Series[0].Samples[0].RawProfile)
	require.NoError(t, err)

	ss := bench.StackCollapseProtoWithOptions(gp, bench.StackCollapseOptions{
		ValueIdx:   0,
		Scale:      1,
		WithLabels: true,
	})
	fmt.Printf("%s \n", strings.Join(ss, "\n"))
	require.Equal(t, 2, len(ss))
	assert.Equal(t, "(process = chrome) ||| chrome.so 0x4e;chrome.so 0x3e 61423", ss[0])
	assert.Equal(t, "(process = firefox) ||| firefox.so 0x2e;firefox.so 0x1e 239", ss[1])
}

type otlpbuilder struct {
	profile   v1experimental.Profile
	stringmap map[string]int64
}

func (o *otlpbuilder) addstr(s string) int64 {
	if o.stringmap == nil {
		o.stringmap = make(map[string]int64)
	}
	if idx, ok := o.stringmap[s]; ok {
		return idx
	}
	idx := int64(len(o.stringmap))
	o.stringmap[s] = idx
	o.profile.StringTable = append(o.profile.StringTable, s)
	return idx
}
