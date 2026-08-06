package pyroscope

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	asyncquery "github.com/grafana/pyroscope/v2/pkg/frontend/async"
)

func TestClassifyProbeErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"conditions not enforced", asyncquery.ErrConditionsNotEnforced, true},
		{"conditions not enforced, wrapped", fmt.Errorf("probe: %w", asyncquery.ErrConditionsNotEnforced), true},
		{"conditions always rejected", asyncquery.ErrConditionsAlwaysRejected, true},
		{"conditions always rejected, wrapped", fmt.Errorf("probe: %w", asyncquery.ErrConditionsAlwaysRejected), true},
		{"transient error", errors.New("bucket unreachable"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyProbeErr(tt.err))
		})
	}
}
