package firedb

import (
	"compress/gzip"
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/prometheus/common/model"
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

var valueTypeStrings = []string{"unit", "type"}

func newValueType() *profilev1.ValueType {
	return &profilev1.ValueType{
		Unit: 1,
		Type: 2,
	}
}

func newProfileFoo() *profilev1.Profile {
	baseTable := append([]string{""}, valueTypeStrings...)
	baseTableLen := int64(len(baseTable)) + 0
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   1,
				Name: baseTableLen + 0,
			},
			{
				Id:   2,
				Name: baseTableLen + 1,
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
			{Id: 1, Filename: baseTableLen + 2},
		},
		StringTable: append(baseTable, []string{
			"func_a",
			"func_b",
			"my-foo-binary",
		}...),
		TimeNanos:  123456,
		PeriodType: newValueType(),
		SampleType: []*profilev1.ValueType{newValueType()},
		Sample: []*profilev1.Sample{
			{
				Value:      []int64{0o123},
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
	baseTable := append([]string{""}, valueTypeStrings...)
	baseTableLen := int64(len(baseTable)) + 0
	return &profilev1.Profile{
		Function: []*profilev1.Function{
			{
				Id:   10,
				Name: baseTableLen + 1,
			},
			{
				Id:   21,
				Name: baseTableLen + 0,
			},
		},
		Location: []*profilev1.Location{
			{
				Id:        1,
				MappingId: 1,
				Address:   0x1337,
				Line: []*profilev1.Line{
					{FunctionId: 10, Line: 1},
				},
			},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: baseTableLen + 2},
		},
		StringTable: append(baseTable, []string{
			"func_b",
			"func_a",
			"my-bar-binary",
		}...),
		TimeNanos:  123456,
		PeriodType: newValueType(),
		SampleType: []*profilev1.ValueType{newValueType()},
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
	head, err := NewHead()
	require.NoError(t, err)

	require.NoError(t, head.Ingest(context.Background(), newProfileFoo()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBaz()))

	require.Equal(t, 3, len(head.functions.slice))
	helper := &functionsHelper{}
	assert.Equal(t, functionsKey{Name: 3}, helper.key(head.functions.slice[0]))
	assert.Equal(t, functionsKey{Name: 4}, helper.key(head.functions.slice[1]))
	assert.Equal(t, functionsKey{Name: 7}, helper.key(head.functions.slice[2]))
}

func TestHeadIngestStrings(t *testing.T) {
	ctx := context.Background()
	head, err := NewHead()
	require.NoError(t, err)

	r := &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileFoo().StringTable, r))
	require.Equal(t, []string{"", "unit", "type", "func_a", "func_b", "my-foo-binary"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 1, 2, 3, 4, 5}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBar().StringTable, r))
	require.Equal(t, []string{"", "unit", "type", "func_a", "func_b", "my-foo-binary", "my-bar-binary"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 1, 2, 4, 3, 6}, r.strings)

	r = &rewriter{}
	require.NoError(t, head.strings.ingest(ctx, newProfileBaz().StringTable, r))
	require.Equal(t, []string{"", "unit", "type", "func_a", "func_b", "my-foo-binary", "my-bar-binary", "func_c"}, head.strings.slice)
	require.Equal(t, stringConversionTable{0, 7}, r.strings)
}

func TestHeadIngestStacktraces(t *testing.T) {
	ctx := context.Background()
	head, err := NewHead()
	require.NoError(t, err)

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
	head, err := NewHead()
	require.NoError(t, err)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.LabelValues(context.Background(), connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "cluster"}))
	require.NoError(t, err)
	require.Equal(t, []string{}, res.Msg.Names)

	res, err = head.LabelValues(context.Background(), connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "job"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, res.Msg.Names)
}

func TestHeadProfileTypes(t *testing.T) {
	head, err := NewHead()
	require.NoError(t, err)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), &commonv1.LabelPair{Name: "__name__", Value: "foo"}, &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), &commonv1.LabelPair{Name: "__name__", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.ProfileTypes(context.Background(), connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar:type:unit:type:unit", "foo:type:unit:type:unit"}, res.Msg.Names)
}

func TestHeadIngestRealProfiles(t *testing.T) {
	profilePaths := []string{
		"testdata/heap",
		"testdata/profile",
	}

	head, err := NewHead()
	require.NoError(t, err)
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

func TestSelectProfiles(t *testing.T) {
	head, err := NewHead()
	require.NoError(t, err)

	// todo write more robust tests.
	for i := int64(0); i < 4; i++ {
		p := newProfileBar()
		p.TimeNanos = int64(time.Second * time.Duration(i))
		err = head.Ingest(context.Background(), p, &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "__name__", Value: "memory"})
		require.NoError(t, err)
	}

	resp, err := head.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
		LabelSelector: `{job="bar"}`,
		Type: &ingestv1.ProfileType{
			Name:       "memory",
			SampleType: "type",
			SampleUnit: "unit",
			PeriodType: "type",
			PeriodUnit: "unit",
		},
		Start: int64(model.TimeFromUnixNano(1 * int64(time.Second))),
		End:   int64(model.TimeFromUnixNano(2 * int64(time.Second))),
	}))
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Msg.Profiles))
	require.Equal(t, 1, len(resp.Msg.FunctionNames))
}

func BenchmarkHeadIngestProfiles(t *testing.B) {
	var (
		profilePaths = []string{
			"testdata/heap",
			"testdata/profile",
		}
		profileCount = 0
	)

	head, err := NewHead()
	require.NoError(t, err)
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
