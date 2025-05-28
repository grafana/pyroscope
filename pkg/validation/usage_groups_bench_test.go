package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/model"
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.GetUsageGroups("tenant1", l)
	}
}

func BenchmarkUsageGroups_Dynamic(b *testing.B) {
	config, err := NewUsageGroupConfig(map[string]string{
		"app/$1":           `{service_name=~"(.*)"}`,
		"team/$1":          `{team=~"(.*)"}`,
		"env/$1":           `{environment=~"(.*)"}`,
		"$1/$2":            `{service_name=~"(.*)", team=~"(.*)"}`,
		"complex/$1-$2-$3": `{service_name=~"(.*)", team=~"(.*)", environment=~"(.*)"}`,
	})
	require.NoError(b, err)

	l := model.Labels{
		{Name: "service_name", Value: "frontend"},
		{Name: "team", Value: "platform"},
		{Name: "environment", Value: "production"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.GetUsageGroups("tenant1", l)
	}
}

func BenchmarkUsageGroups_ComplexRegex(b *testing.B) {
	config, err := NewUsageGroupConfig(map[string]string{
		// Simple regex
		"simple/$1": `{service_name=~"(.*)"}`,
		// More complex regex with character classes
		"complex/$1/$2": `{service_name=~"([a-zA-Z]+)-([0-9]+)"}`,
		// Very complex regex
		"very-complex/$1/$2/$3": `{service_name=~"([a-zA-Z]+)-([0-9]+)\\.([a-z]{2,4})"}`,
	})
	require.NoError(b, err)

	l := model.Labels{
		{Name: "service_name", Value: "frontend-123.prod"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.GetUsageGroups("tenant1", l)
	}
}

func BenchmarkUsageGroups_DynamicParallel(b *testing.B) {
	config, err := NewUsageGroupConfig(map[string]string{
		"app/$1":  `{service_name=~"(.*)"}`,
		"team/$1": `{team=~"(.*)"}`,
	})
	require.NoError(b, err)

	l := model.Labels{
		{Name: "service_name", Value: "frontend"},
		{Name: "team", Value: "platform"},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = config.GetUsageGroups("tenant1", l)
		}
	})
}
