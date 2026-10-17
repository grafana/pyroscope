//go:build helm_unit

package unit

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestLogLevel(t *testing.T) {
	helm, err := exec.LookPath("helm")
	require.NoError(t, err, "helm must be installed to run helm_unit tests")

	for _, mode := range []struct {
		name string
		args []string
	}{
		{name: "v2 single binary"},
		{name: "v1 single binary", args: []string{"--set", "architecture.storage.v1=true,architecture.storage.v2=false"}},
		{name: "v2 microservices", args: []string{"--set", "architecture.microservices.enabled=true"}},
		{name: "v1 microservices", args: []string{"--set", "architecture.microservices.enabled=true,architecture.storage.v1=true,architecture.storage.v2=false"}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				args     []string
				wantFlag string
			}{
				{name: "default"},
				{name: "configured log level", args: []string{"--set", "pyroscope.structuredConfig.server.log_level=warn"}},
				{name: "explicit debug", args: []string{"--set", `pyroscope.extraArgs.log\.level=debug`}, wantFlag: "-log.level=debug"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					args := append([]string{"template", "unit", "..", "--show-only", "templates/deployments-statefulsets.yaml"}, mode.args...)
					args = append(args, tc.args...)
					cmd := exec.CommandContext(ctx, helm, args...)
					var stderr bytes.Buffer
					cmd.Stderr = &stderr
					output, err := cmd.Output()
					require.NoError(t, err, "helm template: %s", stderr.String())

					workloads := 0
					decoder := yaml.NewDecoder(bytes.NewReader(output))
					for {
						var workload struct {
							Kind string `yaml:"kind"`
							Spec struct {
								Template struct {
									Spec struct {
										Containers []struct {
											Args []string `yaml:"args"`
										} `yaml:"containers"`
									} `yaml:"spec"`
								} `yaml:"template"`
							} `yaml:"spec"`
						}
						if err := decoder.Decode(&workload); err != nil {
							if err == io.EOF {
								break
							}
							require.NoError(t, err, "decode rendered YAML")
						}
						if workload.Kind != "StatefulSet" && workload.Kind != "Deployment" {
							continue
						}
						workloads++
						require.NotEmpty(t, workload.Spec.Template.Spec.Containers, "%s has no containers", workload.Kind)
						for _, container := range workload.Spec.Template.Spec.Containers {
							foundLogLevel := false
							for _, arg := range container.Args {
								if strings.HasPrefix(arg, "-log.level=") {
									foundLogLevel = true
									require.Equal(t, tc.wantFlag, arg, "unexpected log-level argument")
								}
							}
							if tc.wantFlag != "" && !foundLogLevel {
								require.True(t, foundLogLevel, "container is missing explicit flag %q", tc.wantFlag)
							}
						}
					}
					require.Positive(t, workloads, "no workloads rendered")
				})
			}
		})
	}
}
