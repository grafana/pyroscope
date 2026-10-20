//go:build helm_unit

package unit

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestPodTemplateLabels(t *testing.T) {
	helm, err := exec.LookPath("helm")
	require.NoError(t, err, "helm must be installed to run helm_unit tests")

	tests := []struct {
		name       string
		values     string
		components []string
		global     map[string]string
		overrides  map[string]string
	}{
		{
			name:       "single binary defaults",
			values:     "{}",
			components: []string{"all"},
		},
		{
			name:       "single binary global labels",
			values:     "pyroscope:\n  extraLabels:\n    team: profiling\n    tier: shared\n",
			components: []string{"all"},
			global:     map[string]string{"team": "profiling", "tier": "shared"},
		},
		{
			name: "component labels without global labels",
			values: `architecture:
  microservices:
    enabled: true
pyroscope:
  components:
    tenant-settings:
      extraLabels:
        singleton: "true"
        numeric: "123"
`,
			components: []string{"tenant-settings", "distributor"},
			overrides:  map[string]string{"singleton": "true", "numeric": "123"},
		},
		{
			name: "component labels merge and remain isolated",
			values: `architecture:
  microservices:
    enabled: true
pyroscope:
  extraLabels:
    team: profiling
    tier: shared
  components:
    tenant-settings:
      extraLabels:
        singleton: "true"
        tier: control-plane
    distributor:
      extraLabels: {}
`,
			components: []string{"tenant-settings", "distributor"},
			global:     map[string]string{"team": "profiling", "tier": "shared"},
			overrides:  map[string]string{"singleton": "true", "tier": "control-plane"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			values := filepath.Join(t.TempDir(), "values.yaml")
			require.NoError(t, os.WriteFile(values, []byte(tt.values), 0600))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, helm, "template", "labels-test", "..", "--values", values,
				"--show-only", "templates/deployments-statefulsets.yaml")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			require.NoError(t, err, "helm template: %s", stderr.String())

			decoder := yaml.NewDecoder(bytes.NewReader(output))
			seen := make(map[string]bool)
			for {
				var workload struct {
					Kind string `yaml:"kind"`
					Spec struct {
						Selector struct {
							MatchLabels map[string]string `yaml:"matchLabels"`
						} `yaml:"selector"`
						Template struct {
							Metadata struct {
								Labels map[string]string `yaml:"labels"`
							} `yaml:"metadata"`
						} `yaml:"template"`
					} `yaml:"spec"`
				}
				err := decoder.Decode(&workload)
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				if workload.Kind != "Deployment" && workload.Kind != "StatefulSet" {
					continue
				}
				labels := workload.Spec.Template.Metadata.Labels
				component := labels["app.kubernetes.io/component"]
				seen[component] = true

				want := map[string]string{
					"app.kubernetes.io/name":      "pyroscope",
					"app.kubernetes.io/instance":  "labels-test",
					"app.kubernetes.io/component": component,
					"name":                        component,
				}
				if component == "all" {
					want["name"] = "pyroscope"
				}
				for k, v := range tt.global {
					want[k] = v
				}
				if component == "tenant-settings" {
					for k, v := range tt.overrides {
						want[k] = v
					}
				}
				require.Equal(t, want, labels, "pod labels for %s", component)
				require.Equal(t, map[string]string{
					"app.kubernetes.io/name":      "pyroscope",
					"app.kubernetes.io/instance":  "labels-test",
					"app.kubernetes.io/component": component,
				}, workload.Spec.Selector.MatchLabels, "selector for %s", component)
			}
			for _, component := range tt.components {
				require.True(t, seen[component], "missing workload %s", component)
			}
		})
	}
}
