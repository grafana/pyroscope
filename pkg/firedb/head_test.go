package firedb

import (
	"compress/gzip"
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

func parseProfile(t testing.TB, path string) *profilev1.Profile {

	f, err := os.Open(path)
	require.NoError(t, err, "failed opening profile: ", path)
	r, err := gzip.NewReader(f)
	require.NoError(t, err)
	content, err := ioutil.ReadAll(r)
	require.NoError(t, err, "failed reading file: ", path)

	p := &profilev1.Profile{}
	require.NoError(t, p.UnmarshalVT(content))

	return p
}

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
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   0x1337,
			},
			{
				Id:        2,
				MappingId: 1,
				Address:   0x1338,
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 3},
		},
		StringTable: []string{
			"",
			"func_a",
			"func_b",
			"my-foo-binary",
		},
		TimeNanos: 123456,
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{0123},
				LocationId: []uint64{1},
			},
			{
				Value:      []int64{1234},
				LocationId: []uint64{1, 2},
			},
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
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   0x1337,
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 3},
		},
		StringTable: []string{
			"",
			"func_b",
			"func_a",
			"my-bar-binary",
		},
		TimeNanos: 123456,
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{2345},
				LocationId: []uint64{1},
			},
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
	assert.Equal(t, functionsKey{Name: 5}, helper.key(head.functions.slice[2]))
}

func TestHeadIngestStrings(t *testing.T) {
	var (
		head = NewHead()
		ctx  = context.Background()
	)

	r := &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileFoo().StringTable, r))
	require.Equal(t, []string{"", "func_a", "func_b", "my-foo-binary"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 1, 2, 3}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBar().StringTable, r))
	require.Equal(t, []string{"", "func_a", "func_b", "my-foo-binary", "my-bar-binary"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 2, 1, 4}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBaz().StringTable, r))
	require.Equal(t, []string{"", "func_a", "func_b", "my-foo-binary", "my-bar-binary", "func_c"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 5}, r.strings)
}

func TestHeadIngestStacktraces(t *testing.T) {
	var (
		head = NewHead()
		ctx  = context.Background()
	)

	require.NoError(t, head.Ingest(ctx, newProfileFoo()))
	require.NoError(t, head.Ingest(ctx, newProfileBar()))
	require.NoError(t, head.Ingest(ctx, newProfileBar()))

	// expect 2 mappings
	require.Equal(t, 2, len(head.mappings.slice))
	assert.Equal(t, "my-foo-binary", head.strings.slice[head.mappings.slice[0].Filename])
	assert.Equal(t, "my-bar-binary", head.strings.slice[head.mappings.slice[1].Filename])

	// expect 3 stacktraces
	require.Equal(t, 3, len(head.stacktraces.slice))

	// expect 3 profiles
	require.Equal(t, 3, len(head.profiles.slice))

	var samples []uint64
	for pos := range head.profiles.slice {
		for _, sample := range head.profiles.slice[pos].Samples {
			samples = append(samples, sample.StacktraceID)
		}
	}
	// expect 4 samples, 3 of which distinct
	require.Equal(t, []uint64{0, 1, 2, 2}, samples)

}

func TestHeadLabelValues(t *testing.T) {
	head := NewHead()
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.LabelValues(context.Background(), connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "cluster"}))
	require.NoError(t, err)
	require.Equal(t, []string{}, res.Msg.Names)

	res, err = head.LabelValues(context.Background(), connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "job"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, res.Msg.Names)

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

	t.Logf("strings=%d samples=%d", len(head.strings.slice), len(head.profiles.slice[0].Samples))
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
			require.NoError(t, head.Ingest(ctx, p))
			profileCount++
		}
	}

}
