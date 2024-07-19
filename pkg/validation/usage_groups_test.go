package validation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUsageGroupConfig(t *testing.T) {
	tests := []struct {
		Name      string
		ConfigMap map[string]string
		Want      UsageGroupConfig
		WantErr   string
	}{
		{
			Name: "single_usage_group",
			ConfigMap: map[string]string{
				"app/foo": `{service_name="foo"}`,
			},
			Want: UsageGroupConfig{
				config: map[string]string{
					"app/foo": `{service_name="foo"}`,
				},
			},
		},
		{
			Name: "multiple_usage_groups",
			ConfigMap: map[string]string{
				"app/foo": `{service_name="foo"}`,
				"app/bar": `{service_name="bar"}`,
			},
			Want: UsageGroupConfig{
				config: map[string]string{
					"app/foo": `{service_name="foo"}`,
					"app/bar": `{service_name="bar"}`,
				},
			},
		},
		{
			Name:      "no_usage_groups",
			ConfigMap: map[string]string{},
			Want:      UsageGroupConfig{},
		},
		{
			Name: "too_many_usage_groups",
			ConfigMap: func() map[string]string {
				m := make(map[string]string)
				for i := 0; i < maxUsageGroups+1; i++ {
					m[fmt.Sprintf("app/foo%d", i)] = `{service_name="foo"}`
				}
				return m
			}(),
			WantErr: fmt.Sprintf("maximum number of usage groups is %d, got %d", maxUsageGroups, maxUsageGroups+1),
		},
	}

	for _, tt := range tests {
		got, err := NewUsageGroupConfig(tt.ConfigMap)
		if tt.WantErr != "" {
			require.EqualError(t, err, tt.WantErr)
		} else {
			require.NoError(t, err)
			require.Equal(t, tt.Want, got)
		}
	}
}
