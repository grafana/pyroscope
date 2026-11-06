//go:build helm_unit

package unit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type resource struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name        string            `yaml:"name"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
	Spec struct {
		ClusterIP string `yaml:"clusterIP"`
		Selector  struct {
			MatchLabels      map[string]string `yaml:"matchLabels"`
			MatchExpressions []struct {
				Key      string   `yaml:"key"`
				Operator string   `yaml:"operator"`
				Values   []string `yaml:"values"`
			} `yaml:"matchExpressions"`
		} `yaml:"selector"`
	} `yaml:"spec"`
}

func TestServiceMonitorExcludesHeadlessServices(t *testing.T) {
	helm, err := exec.LookPath("helm")
	require.NoError(t, err, "helm must be installed to run helm_unit tests")

	for _, microservices := range []bool{false, true} {
		for _, monitoring := range []bool{false, true} {
			for _, annotations := range []bool{false, true} {
				t.Run(fmt.Sprintf("microservices=%t/monitoring=%t/annotations=%t", microservices, monitoring, annotations), func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					args := []string{
						"template", "unit", "..",
						"--show-only", "templates/services.yaml",
						"--set", fmt.Sprintf("architecture.microservices.enabled=%t", microservices),
						"--set", fmt.Sprintf("serviceMonitor.enabled=%t", monitoring),
					}
					if monitoring {
						args = append(args, "--show-only", "templates/servicemonitor.yaml")
					}
					if annotations {
						args = append(args, "--set-string", "pyroscope.service.headlessAnnotations.unit-test=preserved")
					}
					cmd := exec.CommandContext(ctx, helm, args...)
					var stderr bytes.Buffer
					cmd.Stderr = &stderr
					output, err := cmd.Output()
					require.NoError(t, err, "helm template: %s", stderr.String())

					var services, monitors []resource
					decoder := yaml.NewDecoder(bytes.NewReader(output))
					for {
						var doc resource
						err := decoder.Decode(&doc)
						if err == io.EOF {
							break
						}
						require.NoError(t, err)
						switch doc.Kind {
						case "Service":
							services = append(services, doc)
						case "ServiceMonitor":
							monitors = append(monitors, doc)
						}
					}

					headlessCount := 0
					for _, service := range services {
						if !strings.HasSuffix(service.Metadata.Name, "-headless") {
							continue
						}
						headlessCount++
						require.Equal(t, "None", service.Spec.ClusterIP)
						if monitoring {
							require.Equal(t, "false", service.Metadata.Labels["prometheus.io/service-monitor"])
						} else {
							require.NotContains(t, service.Metadata.Labels, "prometheus.io/service-monitor")
						}
						if annotations {
							require.Equal(t, map[string]string{"unit-test": "preserved"}, service.Metadata.Annotations)
						} else {
							require.Empty(t, service.Metadata.Annotations)
						}
					}
					require.Positive(t, headlessCount)
					if !monitoring {
						require.Empty(t, monitors)
						return
					}
					require.Len(t, monitors, headlessCount)
					for _, monitor := range monitors {
						var matches []resource
						for _, service := range services {
							if matchesSelector(t, monitor, service.Metadata.Labels) {
								matches = append(matches, service)
							}
						}
						require.Len(t, matches, 1, "ServiceMonitor %s must select exactly one Service", monitor.Metadata.Name)
						require.Equal(t, monitor.Metadata.Name, matches[0].Metadata.Name)
						require.NotEqual(t, "None", matches[0].Spec.ClusterIP)
					}
				})
			}
		}
	}
}

func matchesSelector(t *testing.T, monitor resource, labels map[string]string) bool {
	t.Helper()
	selector := monitor.Spec.Selector
	for key, value := range selector.MatchLabels {
		if actual, ok := labels[key]; !ok || actual != value {
			return false
		}
	}
	for _, expression := range selector.MatchExpressions {
		// The chart currently uses only NotIn expressions. Fail rather than
		// silently applying incorrect semantics if it introduces another operator.
		require.Equal(t, "NotIn", expression.Operator)
		for _, value := range expression.Values {
			if actual, ok := labels[expression.Key]; ok && actual == value {
				return false
			}
		}
	}
	return true
}
