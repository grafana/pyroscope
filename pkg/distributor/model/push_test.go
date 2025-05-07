package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/distributor/ingest_limits"
)

func TestProfileSeries_GetLanguage(t *testing.T) {
	tests := []struct {
		labels []*typesv1.LabelPair
		want   string
	}{
		{labels: []*typesv1.LabelPair{{Name: "pyroscope_spy", Value: "gospy"}}, want: "go"},
		{labels: []*typesv1.LabelPair{{Name: "pyroscope_spy", Value: "javaspy"}}, want: "java"},
		{labels: []*typesv1.LabelPair{{Name: "pyroscope_spy", Value: "dotnetspy"}}, want: "dotnet"},
		{labels: []*typesv1.LabelPair{{Name: "pyroscope_spy", Value: "grafana-agent.java"}}, want: "java"},
		{labels: []*typesv1.LabelPair{{Name: "pyroscope_spy", Value: ""}}, want: ""},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			p := &ProfileSeries{
				Labels: tt.labels,
			}
			if got := p.GetLanguage(); got != tt.want {
				t.Errorf("GetLanguage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarkThrottledTenant(t *testing.T) {
	tests := []struct {
		name        string
		req         *PushRequest
		limit       *ingest_limits.Config
		expectError bool
		verify      func(t *testing.T, req *PushRequest)
	}{
		{
			name: "single series",
			req: &PushRequest{
				Series: []*ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
						},
					},
				},
			},
			limit: &ingest_limits.Config{
				PeriodType:     "hour",
				PeriodLimitMb:  128,
				LimitResetTime: time.Now().Unix(),
				LimitReached:   true,
			},
			verify: func(t *testing.T, req *PushRequest) {
				require.Len(t, req.Series[0].Annotations, 1)
				assert.Equal(t, ingest_limits.ProfileAnnotationKeyThrottled, req.Series[0].Annotations[0].Key)
				assert.Contains(t, req.Series[0].Annotations[0].Value, "\"periodLimitMb\":128")
			},
		},
		{
			name: "multiple series",
			req: &PushRequest{
				Series: []*ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
						},
					},
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "memory"},
						},
					},
				},
			},
			limit: &ingest_limits.Config{
				PeriodType:     "hour",
				PeriodLimitMb:  128,
				LimitResetTime: time.Now().Unix(),
				LimitReached:   true,
			},
			verify: func(t *testing.T, req *PushRequest) {
				for _, series := range req.Series {
					require.Len(t, series.Annotations, 1)
					assert.Equal(t, ingest_limits.ProfileAnnotationKeyThrottled, series.Annotations[0].Key)
					assert.Contains(t, series.Annotations[0].Value, "\"periodLimitMb\":128")
				}
			},
		},
		{
			name: "invalid limit config",
			req: &PushRequest{
				Series: []*ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
						},
					},
				},
			},
			limit:       nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.MarkThrottledTenant(tt.limit)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.verify != nil {
				tt.verify(t, tt.req)
			}
		})
	}
}

func TestMarkThrottledUsageGroup(t *testing.T) {
	tests := []struct {
		name        string
		req         *PushRequest
		limit       *ingest_limits.Config
		usageGroup  string
		expectError bool
		verify      func(t *testing.T, req *PushRequest)
	}{
		{
			name: "single series with usage group",
			req: &PushRequest{
				Series: []*ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
						},
					},
				},
			},
			limit: &ingest_limits.Config{
				PeriodType:     "hour",
				PeriodLimitMb:  128,
				LimitResetTime: time.Now().Unix(),
				LimitReached:   true,
				UsageGroups: map[string]ingest_limits.UsageGroup{
					"group-1": {
						PeriodLimitMb: 64,
						LimitReached:  true,
					},
				},
			},
			usageGroup: "group-1",
			verify: func(t *testing.T, req *PushRequest) {
				require.Len(t, req.Series[0].Annotations, 1)
				assert.Equal(t, ingest_limits.ProfileAnnotationKeyThrottled, req.Series[0].Annotations[0].Key)
				assert.Contains(t, req.Series[0].Annotations[0].Value, "\"periodLimitMb\":64")
				assert.Contains(t, req.Series[0].Annotations[0].Value, "\"usageGroup\":\"group-1\"")
			},
		},
		{
			name: "invalid usage group",
			req: &PushRequest{
				Series: []*ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
						},
					},
				},
			},
			limit: &ingest_limits.Config{
				PeriodType:     "hour",
				PeriodLimitMb:  128,
				LimitResetTime: time.Now().Unix(),
				LimitReached:   true,
				UsageGroups: map[string]ingest_limits.UsageGroup{
					"group-1": {
						PeriodLimitMb: 64,
						LimitReached:  true,
					},
				},
			},
			usageGroup:  "nonexistent-group",
			expectError: true,
		},
		{
			name: "invalid limit config",
			req: &PushRequest{
				Series: []*ProfileSeries{
					{
						Labels: []*typesv1.LabelPair{
							{Name: "__name__", Value: "cpu"},
						},
					},
				},
			},
			limit:       nil,
			usageGroup:  "group-1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.MarkThrottledUsageGroup(tt.limit, tt.usageGroup)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.verify != nil {
				tt.verify(t, tt.req)
			}
		})
	}
}
