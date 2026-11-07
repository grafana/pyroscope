package pprof

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
)

func TestOwnedBytesMatchesCloningMerge(t *testing.T) {
	files, err := filepath.Glob("testdata/*.pprof")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			encoded, err := os.ReadFile(file)
			require.NoError(t, err)
			for _, sanitize := range []bool{false, true} {
				var cloned, owned ProfileMerge
				for range 3 {
					var p profilev1.Profile
					require.NoError(t, Unmarshal(encoded, &p))
					require.NoError(t, cloned.Merge(&p, sanitize))
					require.NoError(t, owned.MergeBytes(encoded, sanitize))
				}
				testhelper.EqualProto(t, cloned.Profile(), owned.Profile())
			}
		})
	}
}

func TestOwnedBytesMixedWithCloningMerge(t *testing.T) {
	encoded, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var expected, actual ProfileMerge
	for i := range 4 {
		var p profilev1.Profile
		require.NoError(t, Unmarshal(encoded, &p))
		require.NoError(t, expected.Merge(p.CloneVT(), true))
		if i%2 == 0 {
			require.NoError(t, actual.MergeBytes(encoded, true))
		} else {
			require.NoError(t, actual.Merge(&p, true))
			// The public Merge method must continue to copy caller-owned data.
			for _, sample := range p.Sample {
				clear(sample.LocationId)
				clear(sample.Value)
			}
			for _, loc := range p.Location {
				loc.Address = 0
				clear(loc.Line)
			}
			for _, fn := range p.Function {
				fn.Name = 0
			}
		}
	}
	testhelper.EqualProto(t, expected.Profile(), actual.Profile())
}

func TestOwnedBytesConcurrent(t *testing.T) {
	encoded, err := os.ReadFile("testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var expected, actual ProfileMerge
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		require.NoError(t, expected.MergeBytes(encoded, true))
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- actual.MergeBytes(encoded, true)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	testhelper.EqualProto(t, expected.Profile(), actual.Profile())
}
