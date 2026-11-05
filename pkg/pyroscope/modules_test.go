package pyroscope

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every architecture must leave exactly one kind of seed writer; every other target follows.
func TestInitUsageReport_SeedLeader(t *testing.T) {
	for _, tc := range []struct {
		architecture string
		target       string
		wantLeader   bool
	}{
		{architecture: "v2", target: All, wantLeader: true},
		{architecture: "v2", target: SegmentWriter, wantLeader: true},
		// Distributor reaches SegmentWriterRing via SegmentWriterClient, but never SegmentWriter.
		{architecture: "v2", target: Distributor, wantLeader: false},

		{architecture: "v1-v2-dual", target: Ingester, wantLeader: true},
		{architecture: "v1-v2-dual", target: SegmentWriter, wantLeader: true},

		// v1 drops the segment writer from All, leaving the ingester.
		{architecture: "v1", target: All, wantLeader: true},
		{architecture: "v1", target: Distributor, wantLeader: false},
	} {
		t.Run(tc.architecture+"/"+tc.target, func(t *testing.T) {
			f := &Pyroscope{
				Cfg:    newTestConfig(t, []string{"-architecture.storage=" + tc.architecture, "-target=" + tc.target}),
				logger: &logger{Logger: log.NewNopLogger()},
			}
			// Without a configured object store, initUsageReport writes a filesystem bucket here.
			f.Cfg.PhlareDB.DataPath = t.TempDir()
			require.NoError(t, f.setupModuleManager())

			svc, err := f.initUsageReport()
			require.NoError(t, err)
			require.NotNil(t, svc, "usage report service should be constructed")

			assert.Equal(t, tc.wantLeader, f.Cfg.Analytics.Leader)
		})
	}
}
