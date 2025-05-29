package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util"
)

func BenchmarkUsageGroups_Regular(b *testing.B) {
	config, err := NewUsageGroupConfig(map[string]string{
		"app/frontend":  `{service_name=~"(.*)"}`,
		"app/backend":   `{team=~"(.*)"}`,
		"app/database":  `{environment=~"(.*)"}`,
		"team/platform": `{service_name=~"(.*)", team=~"(.*)"}`,
		"team/product":  `{service_name=~"(.*)", team=~"(.*)", environment=~"(.*)"}`,
	})
	require.NoError(b, err)

	l := model.Labels{
		{Name: "service_name", Value: "frontend"},
		{Name: "team", Value: "platform"},
		{Name: "environment", Value: "production"},
	}
	evaluator := NewUsageGroupEvaluator(util.Logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluator.GetMatch("tenant1", config, l)
	}
}

func BenchmarkUsageGroups_Dynamic(b *testing.B) {
	config, err := NewUsageGroupConfig(map[string]string{
		"app/${labels.service_name}":                                          `{service_name=~".*"}`,
		"team/${labels.team}":                                                 `{team=~".*"}`,
		"env/${labels.environment}":                                           `{environment=~".*"}`,
		"${labels.service_name}/${labels.team}":                               `{service_name=~".*", team=~".*"}`,
		"complex/${labels.service_name}-${labels.team}-${labels.environment}": `{service_name=~".*", team=~".*", environment=~".*"}`,
	})
	require.NoError(b, err)

	l := model.Labels{
		{Name: "service_name", Value: "frontend"},
		{Name: "team", Value: "platform"},
		{Name: "environment", Value: "production"},
	}
	evaluator := NewUsageGroupEvaluator(util.Logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluator.GetMatch("tenant1", config, l)
	}
}

func BenchmarkUsageGroups_ComplexRegex(b *testing.B) {
	config, err := NewUsageGroupConfig(map[string]string{
		"complex/${labels.service_name}":      `{service_name=~"[a-zA-Z]+-[0-9]+"}`,
		"very-complex/${labels.service_name}": `{service_name=~"[a-zA-Z]+-[0-9]+\\.[a-z]{2,4}"}`,
	})
	require.NoError(b, err)

	l := model.Labels{
		{Name: "service_name", Value: "frontend-123.prod"},
	}
	evaluator := NewUsageGroupEvaluator(util.Logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluator.GetMatch("tenant1", config, l)
	}
}
