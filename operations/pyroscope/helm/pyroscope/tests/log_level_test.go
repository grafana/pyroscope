package tests

import (
	"os/exec"
	"strings"
	"testing"
)

func TestLogLevel(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is required to render the chart")
	}

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
					args := append([]string{"template", "test", "..", "--show-only", "templates/deployments-statefulsets.yaml"}, mode.args...)
					args = append(args, tc.args...)
					output, err := exec.Command(helm, args...).CombinedOutput()
					if err != nil {
						t.Fatalf("helm template: %v\n%s", err, output)
					}
					// Check every workload, so microservices cannot silently retain a default override.
					workloads := 0
					for _, workload := range strings.Split(string(output), "\n---\n") {
						if !strings.Contains(workload, "\nkind: StatefulSet\n") && !strings.Contains(workload, "\nkind: Deployment\n") {
							continue
						}
						workloads++
						if tc.wantFlag == "" {
							if strings.Contains(workload, "-log.level=") {
								t.Error("chart overrides the configured or server default log level")
							}
						} else if !strings.Contains(workload, tc.wantFlag) {
							t.Errorf("workload is missing explicit flag %q", tc.wantFlag)
						}
					}
					if workloads == 0 {
						t.Fatal("no workloads rendered")
					}
				})
			}
		})
	}
}
