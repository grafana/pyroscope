//go:build helm_unit

package unit

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestArchitectureValidation(t *testing.T) {
	helm, err := exec.LookPath("helm")
	require.NoError(t, err, "helm must be installed to run helm_unit tests")

	const (
		v1Only = "architecture.storage.v1=true,architecture.storage.v2=false"
		dual   = "architecture.storage.v1=true,architecture.storage.v2=true"
		m      = "architecture.storage.migration."
	)
	tests := []struct {
		name    string
		set     string
		wantErr string
	}{
		{name: "v2 defaults", set: "architecture.storage.v2=true"},
		{name: "v1 only", set: v1Only},
		{name: "v1 only ignores migration", set: v1Only + "," + m + "ingesterWeight=5," + m + "queryBackendFrom=junk"},
		{name: "dual defaults", set: dual},
		{name: "dual microservices", set: dual + ",architecture.microservices.enabled=true"},
		{name: "dual ingester primary", set: dual + "," + m + "ingesterWeight=1," + m + "segmentWriterWeight=0.1"},
		{name: "dual segment writer only", set: dual + "," + m + "ingesterWeight=0," + m + "segmentWriterWeight=1"},
		{name: "dual query backend from date", set: dual + "," + m + "queryBackendFrom=2024-01-02"},
		{name: "dual query backend from rfc3339", set: dual + "," + m + "queryBackendFrom=2024-01-02T15:04:05Z"},
		{
			name:    "no storage",
			set:     "architecture.storage.v1=false,architecture.storage.v2=false",
			wantErr: "you need to set at least one of .Values.architecture.storage.v1, .Values.architecture.storage.v2 to true",
		},
		{
			name:    "weights both zero",
			set:     dual + "," + m + "ingesterWeight=0," + m + "segmentWriterWeight=0",
			wantErr: "otherwise some writes are dropped (got 0 and 0)",
		},
		{
			name:    "weights below one",
			set:     dual + "," + m + "ingesterWeight=0.5," + m + "segmentWriterWeight=0.5",
			wantErr: "otherwise some writes are dropped (got 0.5 and 0.5)",
		},
		{
			name:    "weight above one",
			set:     dual + "," + m + "segmentWriterWeight=1.5",
			wantErr: "segmentWriterWeight must be between 0 and 1, got 1.5",
		},
		{
			name:    "negative weight",
			set:     dual + "," + m + "ingesterWeight=-1",
			wantErr: "ingesterWeight must be between 0 and 1, got -1",
		},
		{
			name:    "weight not a number",
			set:     dual + "," + m + "ingesterWeight=half",
			wantErr: `ingesterWeight must be a number between 0 and 1, got "half"`,
		},
		{
			name:    "invalid query backend from",
			set:     dual + "," + m + "queryBackendFrom=yesterday",
			wantErr: `queryBackendFrom must be "auto" or a time like`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, helm, "template", "unit", "..", "--set", tt.set)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			_, err := cmd.Output()
			if tt.wantErr == "" {
				require.NoError(t, err, "helm template: %s", stderr.String())
				return
			}
			require.Error(t, err)
			require.Contains(t, stderr.String(), tt.wantErr)
		})
	}
}
