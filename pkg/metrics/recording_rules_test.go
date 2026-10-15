package metrics

import (
	"bytes"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

func TestRecordingRulesConfig_ReconcileWithExporter(t *testing.T) {
	const addr = "settings.svc:4040"
	const legacyAddr = "legacy.svc:4040"

	for _, tc := range []struct {
		name        string
		shared      RecordingRulesConfig
		legacyOn    bool
		legacyAddr  string
		wantEnabled bool
		wantAddress string
	}{
		{
			name:        "unset -> disabled, static",
			wantEnabled: false,
			wantAddress: "",
		},
		{
			name:        "shared flags only",
			shared:      RecordingRulesConfig{Enabled: true, ClientAddress: addr},
			wantEnabled: true,
			wantAddress: addr,
		},
		{
			name:        "deprecated flags only are honored",
			legacyOn:    true,
			legacyAddr:  legacyAddr,
			wantEnabled: true,
			wantAddress: legacyAddr,
		},
		{
			name:        "shared address wins over deprecated",
			shared:      RecordingRulesConfig{Enabled: true, ClientAddress: addr},
			legacyOn:    true,
			legacyAddr:  legacyAddr,
			wantEnabled: true,
			wantAddress: addr,
		},
		{
			name:        "shared enabled, static (no address)",
			shared:      RecordingRulesConfig{Enabled: true},
			wantEnabled: true,
			wantAddress: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exporter := Config{Enabled: tc.legacyOn}
			exporter.RulesSource.ClientAddress = tc.legacyAddr
			shared := tc.shared

			var logs bytes.Buffer
			shared.ReconcileWithExporter(&exporter, log.NewLogfmtLogger(&logs))

			if tc.legacyOn || tc.legacyAddr != "" {
				assert.Contains(t, logs.String(), "deprecated")
			} else {
				assert.Empty(t, logs.String())
			}

			// Shared and deprecated fields both hold the effective values.
			assert.Equal(t, tc.wantEnabled, shared.Enabled)
			assert.Equal(t, tc.wantEnabled, exporter.Enabled)
			assert.Equal(t, tc.wantAddress, shared.ClientAddress)
			assert.Equal(t, tc.wantAddress, exporter.RulesSource.ClientAddress)
		})
	}
}
