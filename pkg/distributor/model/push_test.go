package model

import (
	"testing"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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
