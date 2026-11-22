//go:build helm_unit

package unit

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueryBackendAddressSrvPrefix(t *testing.T) {
	helm, err := exec.LookPath("helm")
	require.NoError(t, err, "helm must be installed to run helm_unit tests")

	for _, tc := range []struct {
		useSrvPrefix bool
		want         string
	}{
		{false, `"-query-backend.address=dns:///unit-pyroscope-headless.$(NAMESPACE_FQDN):9095"`},
		{true, `"-query-backend.address=dns:///_grpc._tcp.unit-pyroscope-headless.$(NAMESPACE_FQDN):9095"`},
	} {
		t.Run(fmt.Sprintf("useSrvPrefix=%t", tc.useSrvPrefix), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, helm, "template", "unit", "..",
				"--show-only", "templates/deployments-statefulsets.yaml",
				"--set", fmt.Sprintf("pyroscope.queryBackend.useSrvPrefix=%t", tc.useSrvPrefix),
			).CombinedOutput()
			require.NoError(t, err, string(out))
			require.Contains(t, string(out), tc.want)
		})
	}
}
