package phlaredb_test

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/block/testutil"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func Test_ValidateBlock(t *testing.T) {
	meta, dir := testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(1)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})

	err := phlaredb.ValidateLocalBlock(context.Background(), path.Join(dir, meta.ULID.String()))
	require.NoError(t, err)
	t.Run("should error when a file is missing", func(t *testing.T) {
		os.Remove(path.Join(dir, meta.ULID.String(), block.IndexFilename))
		err = phlaredb.ValidateLocalBlock(context.Background(), path.Join(dir, meta.ULID.String()))
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such file")
	})
}
