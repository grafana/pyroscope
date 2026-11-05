package tree

import (
	"testing"
	"time"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func setupPprofTree(t *testing.T) (*Tree, *googlev1.Profile, []string) {
	t.Helper()

	tr := New()
	tr.Insert([]byte("main;foo;1;2;3;4;5"), uint64(15))
	tr.Insert([]byte("main;baz;1;2;3;4;5;6"), uint64(55))
	tr.Insert([]byte("main;baz;1;2;3;4;5;6;8"), uint64(25))
	tr.Insert([]byte("main;bar;1;2;3;4;5;6;8"), uint64(35))
	tr.Insert([]byte("main;foo;1;2;3;4;5;6"), uint64(20))
	tr.Insert([]byte("main;bar;1;2;3;4;5;6;9"), uint64(35))
	tr.Insert([]byte("main;baz;1;2;3;4;5;6;7"), uint64(30))
	tr.Insert([]byte("main;qux;1;2;3;4;5;6;7;8;9"), uint64(65))

	profile := tr.Pprof(&PprofMetadata{
		Type:      "cpu",
		Unit:      "samples",
		StartTime: time.Date(2021, 1, 1, 10, 0, 1, 0, time.UTC),
		Duration:  time.Minute,
	})
	fnNames := []string{
		"main", "foo", "bar", "baz", "qux",
		"1", "2", "3", "4", "5", "6", "7", "8", "9",
	}
	return tr, profile, fnNames
}

func TestPprof(t *testing.T) {
	tree, profile, fnNames := setupPprofTree(t)

	t.Run("Should serialize correctly", func(t *testing.T) {
		_, err := proto.Marshal(profile)
		require.NoError(t, err)
	})

	t.Run("StringTable Should build correctly", func(t *testing.T) {
		require.Len(t, profile.StringTable, 17)
		assert.ElementsMatch(t, append(fnNames, "", "cpu", "samples"), profile.StringTable)
	})

	t.Run("StringTable Should have empty first element", func(t *testing.T) {
		profile := tree.Pprof(&PprofMetadata{})
		require.Equal(t, "", profile.StringTable[0])
	})

	t.Run("Metadata Should build correctly", func(t *testing.T) {
		require.Equal(t, int64(1609495201000000000), profile.TimeNanos)
		require.Equal(t, int64(60000000000), profile.DurationNanos)
		require.Len(t, profile.SampleType, 1)
		_type := profile.StringTable[profile.SampleType[0].Type]
		unit := profile.StringTable[profile.SampleType[0].Unit]
		require.Equal(t, "cpu", _type)
		require.Equal(t, "samples", unit)
	})

	t.Run("Function Should build correctly", func(t *testing.T) {
		require.Len(t, profile.Function, 14)
		for _, fn := range profile.Function {
			require.NotZero(t, fn.Id)
			require.NotZero(t, fn.Name)
			require.NotZero(t, fn.SystemName)
		}
	})

	t.Run("Function Name Should have corresponding StringTable entries", func(t *testing.T) {
		var names []string
		for _, fn := range profile.Function {
			names = append(names, profile.StringTable[fn.Name])
		}
		assert.ElementsMatch(t, fnNames, names)
	})

	t.Run("Function SystemName Should have corresponding StringTable entries", func(t *testing.T) {
		var names []string
		for _, fn := range profile.Function {
			names = append(names, profile.StringTable[fn.SystemName])
		}
		assert.ElementsMatch(t, fnNames, names)
	})

	t.Run("Location Should build correctly", func(t *testing.T) {
		require.Len(t, profile.Location, 14)
		for _, l := range profile.Location {
			require.NotZero(t, l.Id)
			require.NotEmpty(t, l.Line)
			require.NotZero(t, l.Line[0].FunctionId)
		}
	})

	t.Run("Location Should have corresponding functions", func(t *testing.T) {
		fnMap := make(map[uint64]*googlev1.Function)
		for _, fn := range profile.Function {
			fnMap[fn.Id] = fn
		}
		for _, l := range profile.Location {
			_, ok := fnMap[l.Line[0].FunctionId]
			require.True(t, ok)
		}
	})

	t.Run("Sample Should build correctly", func(t *testing.T) {
		require.Len(t, profile.Sample, 8)
		for _, s := range profile.Sample {
			require.NotEmpty(t, s.LocationId)
			require.NotEmpty(t, s.Value)
		}
	})

	t.Run("Sample Should have corresponding locations", func(t *testing.T) {
		lMap := make(map[uint64]*googlev1.Location)
		for _, l := range profile.Location {
			lMap[l.Id] = l
		}
		for _, s := range profile.Sample {
			for _, l := range s.LocationId {
				_, ok := lMap[l]
				require.True(t, ok)
			}
		}
	})
}
