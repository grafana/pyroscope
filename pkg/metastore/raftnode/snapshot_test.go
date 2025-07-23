package raftnode

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/test"
)

func TestCopySnapshots(t *testing.T) {
	tmp := t.TempDir()
	snapshotsImportDir := filepath.Join(tmp, "src")
	snapshotsDir := filepath.Join(tmp, "dst")
	test.Copy(t, "testdata/snapshots", snapshotsImportDir+"/snapshots")

	buf := bytes.NewBuffer(nil)
	n := &Node{
		logger: log.NewLogfmtLogger(buf),
		config: Config{
			SnapshotsImportDir: snapshotsImportDir,
			SnapshotsDir:       snapshotsDir,
		},
	}

	require.NoError(t, n.importSnapshots())
	require.NoError(t, n.importSnapshots())

	actual := bytes.NewBuffer(nil)
	for _, line := range []string{
		`level=info msg="importing snapshots"`,
		`level=info msg="importing snapshot" snapshot=/tmp/src/snapshots/81-206944-1744474737935`,
		`level=info msg="importing snapshot" snapshot=/tmp/src/snapshots/82-215276-1744546508773`,
		`level=info msg="importing snapshot" snapshot=/tmp/src/snapshots/83-223473-1744577537873`,
		`level=info msg="importing snapshots"`,
	} {
		_, _ = fmt.Fprintln(actual, line)
	}

	assert.Equal(t,
		strings.ReplaceAll(buf.String(), n.config.SnapshotsImportDir, "/tmp/src"),
		actual.String(),
	)
}
