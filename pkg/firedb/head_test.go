package firedb

import (
	"context"
	"testing"
	"time"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newProfileFoo() *profilev1.Profile {
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: 1,
			},
			{
				Id:   2,
				Name: 2,
			},
		},
		StringTable: []string{
			"",
			"func_a",
			"func_b",
		},
	}
}

func newProfileBar() *profilev1.Profile {
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   10,
				Name: 2,
			},
			{
				Id:   21,
				Name: 1,
			},
		},
		StringTable: []string{
			"",
			"func_b",
			"func_a",
		},
	}
}

func newProfileBaz() *profilev1.Profile {
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   25,
				Name: 1,
			},
		},
		StringTable: []string{
			"",
			"func_c",
		},
	}
}

func TestHeadIngestFunctions(t *testing.T) {
	head := NewHead()

	require.NoError(t, head.Ingest(context.Background(), newProfileFoo()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBaz()))

	require.Equal(t, 3, len(head.functions.slice))
	helper := &functionsHelper{}
	assert.Equal(t, functionsKey{Name: 1}, helper.key(head.functions.slice[0]))
	assert.Equal(t, functionsKey{Name: 2}, helper.key(head.functions.slice[1]))
	assert.Equal(t, functionsKey{Name: 3}, helper.key(head.functions.slice[2]))
}

func TestHeadIngestStrings(t *testing.T) {
	var (
		head = NewHead()
		ctx  = context.Background()
	)

	r := &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileFoo().StringTable, r))
	require.Equal(t, []string{"", "func_a", "func_b"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 1, 2}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBar().StringTable, r))
	require.Equal(t, []string{"", "func_a", "func_b"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 2, 1}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBaz().StringTable, r))
	require.Equal(t, []string{"", "func_a", "func_b", "func_c"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 3}, r.strings)
}

func TestHeadIngestRealProfiles(t *testing.T) {
	var (
		profilePaths = []string{
			"testdata/heap",
			"testdata/profile",
		}
	)

	head := NewHead()
	ctx := context.Background()

	for range make([]struct{}, 100) {
		for pos := range profilePaths {
			profile := parseProfile(t, profilePaths[pos])
			require.NoError(t, head.Ingest(ctx, profile))
		}
	}

	require.NoError(t, head.WriteTo(ctx, t.TempDir()))

	time.Sleep(time.Hour)

	t.Logf("strings=%d samples=%d", len(head.strings.slice), len(head.samples.slice[0].Values))
}

func TestDelta(t *testing.T) {

	sc := parquet.SchemaOf(struct {
		Values []struct {
			Value int `parquet:",delta"`
		}
	}{})

	c, err := sc.Lookup("values", "value")

	t.Logf("%+#v %+#v", c, err)

	sc2 := parquet.SchemaOf(struct {
		Values []int `parquet:","`
	}{})
	t.Logf("%s", sc2)

}

func BenchmarkHeadIngestProfiles(t *testing.B) {
	var (
		profilePaths = []string{
			"testdata/heap",
			"testdata/profile",
		}
		profileCount = 0
	)

	head := NewHead()
	ctx := context.Background()

	t.ReportAllocs()

	for n := 0; n < t.N; n++ {
		for pos := range profilePaths {
			p := parseProfile(t, profilePaths[pos])
			head.Ingest(ctx, p)
			profileCount++
		}
	}

}
