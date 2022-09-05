package firedb

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/google/uuid"
	"github.com/klauspost/compress/gzip"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	firemodel "github.com/grafana/fire/pkg/model"
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

func newEmptyProfile() *profilev1.Profile {
	p := newProfileBar()
	for _, s := range p.Sample {
		for i := range s.Value {
			s.Value[i] = 0
		}
	}
	return p
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
				Id:        113,
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
				LocationId: []uint64{113},
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
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBaz(), uuid.New()))

	require.Equal(t, 3, len(head.functions.slice))
	helper := &functionsHelper{}
	assert.Equal(t, functionsKey{Name: 3}, helper.key(head.functions.slice[0]))
	assert.Equal(t, functionsKey{Name: 4}, helper.key(head.functions.slice[1]))
	assert.Equal(t, functionsKey{Name: 7}, helper.key(head.functions.slice[2]))
}

func TestHeadIngestStrings(t *testing.T) {
	ctx := context.Background()
	head, err := NewHead(t.TempDir())
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
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, head.Ingest(ctx, newProfileFoo(), uuid.New()))
	require.NoError(t, head.Ingest(ctx, newProfileBar(), uuid.New()))
	require.NoError(t, head.Ingest(ctx, newProfileBar(), uuid.New()))

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
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.LabelValues(context.Background(), connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "cluster"}))
	require.NoError(t, err)
	require.Equal(t, []string{}, res.Msg.Names)

	res, err = head.LabelValues(context.Background(), connect.NewRequest(&ingestv1.LabelValuesRequest{Name: "job"}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, res.Msg.Names)
}

func TestHeadLabelNames(t *testing.T) {
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.LabelNames(context.Background(), connect.NewRequest(&ingestv1.LabelNamesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"__period_type__", "__period_unit__", "__profile_type__", "__type__", "__unit__", "job", "namespace"}, res.Msg.Names)
}

func TestHeadSeries(t *testing.T) {
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	fooLabels := firemodel.NewLabelsBuilder(nil).Set("namespace", "fire").Set("job", "foo").Labels()
	barLabels := firemodel.NewLabelsBuilder(nil).Set("namespace", "fire").Set("job", "bar").Labels()
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), fooLabels...))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), barLabels...))

	expected := firemodel.NewLabelsBuilder(nil).
		Set("namespace", "fire").
		Set("job", "foo").
		Set("__period_type__", "type").
		Set("__period_unit__", "unit").
		Set("__type__", "type").
		Set("__unit__", "unit").
		Set("__profile_type__", ":type:unit:type:unit").
		Labels()
	res, err := head.Series(context.Background(), connect.NewRequest(&ingestv1.SeriesRequest{Matchers: []string{`{job="foo"}`}}))
	require.NoError(t, err)
	require.Equal(t, []*commonv1.Labels{{Labels: expected}}, res.Msg.LabelsSet)
}

func TestHeadProfileTypes(t *testing.T) {
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &commonv1.LabelPair{Name: "__name__", Value: "foo"}, &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &commonv1.LabelPair{Name: "__name__", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.ProfileTypes(context.Background(), connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []*commonv1.ProfileType{
		mustParseProfileSelector(t, "bar:type:unit:type:unit"),
		mustParseProfileSelector(t, "foo:type:unit:type:unit"),
	}, res.Msg.ProfileTypes)
}

func mustParseProfileSelector(t testing.TB, selector string) *commonv1.ProfileType {
	ps, err := firemodel.ParseProfileTypeSelector(selector)
	require.NoError(t, err)
	return ps
}

func TestHeadIngestRealProfiles(t *testing.T) {
	profilePaths := []string{
		"testdata/heap",
		"testdata/profile",
	}

	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	for pos := range profilePaths {
		profile := parseProfile(t, profilePaths[pos])
		require.NoError(t, head.Ingest(ctx, profile, uuid.New()))
	}

	require.NoError(t, head.Flush(ctx))
	t.Logf("strings=%d samples=%d", len(head.strings.slice), len(head.profiles.slice[0].Samples))
}

func TestSelectProfiles(t *testing.T) {
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)

	// todo write more robust tests.
	for i := int64(0); i < 4; i++ {
		pF := newProfileFoo()
		pB := newProfileBar()
		pE := newEmptyProfile()
		pF.TimeNanos = int64(time.Second * time.Duration(i))
		pE.TimeNanos = int64(time.Second * time.Duration(i))
		pB.TimeNanos = int64(time.Second * time.Duration(i))
		err = head.Ingest(context.Background(), pF, uuid.New(), &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "__name__", Value: "foomemory"})
		require.NoError(t, err)
		err = head.Ingest(context.Background(), pB, uuid.New(), &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "__name__", Value: "memory"})
		require.NoError(t, err)
		err = head.Ingest(context.Background(), pE, uuid.New(), &commonv1.LabelPair{Name: "job", Value: "bar"}, &commonv1.LabelPair{Name: "__name__", Value: "memory"})
		require.NoError(t, err)
	}

	resp, err := head.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
		LabelSelector: `{job="bar"}`,
		Type: &commonv1.ProfileType{
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

	// compare the first profile deep
	profileJSON, err := json.Marshal(&resp.Msg.Profiles[0])
	require.NoError(t, err)
	require.JSONEq(t, `{
  "type": {
    "name": "memory",
    "sample_type": "type",
    "sample_unit": "unit",
    "period_type": "type",
    "period_unit": "unit"
  },
  "ID":"`+resp.Msg.Profiles[0].ID+`",
  "labels": [
    {
      "name": "__name__",
      "value": "memory"
    },
    {
      "name": "__period_type__",
      "value": "type"
    },
    {
      "name": "__period_unit__",
      "value": "unit"
    },
    {
      "name": "__profile_type__",
      "value": "memory:type:unit:type:unit"
    },
    {
      "name": "__type__",
      "value": "type"
    },
    {
      "name": "__unit__",
      "value": "unit"
    },
    {
      "name": "job",
      "value": "bar"
    }
  ],
  "timestamp": 1000,
  "stacktraces": [
    {
      "function_ids": [
        0
      ],
      "value": 2345
    }
  ]}`, string(profileJSON))

	// ensure the func name matches
	require.Equal(t, []string{"func_a"}, resp.Msg.FunctionNames)
}

func BenchmarkHeadIngestProfiles(t *testing.B) {
	var (
		profilePaths = []string{
			"testdata/heap",
			"testdata/profile",
		}
		profileCount = 0
	)

	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	t.ReportAllocs()

	for n := 0; n < t.N; n++ {
		for pos := range profilePaths {
			p := parseProfile(t, profilePaths[pos])
			require.NoError(t, head.Ingest(ctx, p, uuid.New()))
			profileCount++
		}
	}
}

var res *connect.Response[ingestv1.SelectProfilesResponse]

func BenchmarkSelectProfile(b *testing.B) {
	head, err := NewHead(b.TempDir())
	require.NoError(b, err)
	ctx := context.Background()

	p := parseProfile(b, "testdata/heap")
	for i := 0; i < 10; i++ {
		p.TimeNanos = int64(time.Second * time.Duration(i))
		require.NoError(b,
			head.Ingest(ctx, p, uuid.New(),
				&commonv1.LabelPair{
					Name:  "job",
					Value: "bar",
				}, &commonv1.LabelPair{
					Name:  model.MetricNameLabel,
					Value: "memory",
				}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		res, err = head.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
			LabelSelector: `{job="bar"}`,
			Type: &commonv1.ProfileType{
				ID:         "memory:alloc_space:bytes:space:bytes",
				Name:       "memory",
				SampleType: "alloc_space",
				SampleUnit: "bytes",
				PeriodType: "space",
				PeriodUnit: "bytes",
			},
			Start: int64(model.Earliest),
			End:   int64(model.Latest),
		}))
		require.NoError(b, err)
		res, err = head.SelectProfiles(context.Background(), connect.NewRequest(&ingestv1.SelectProfilesRequest{
			LabelSelector: `{job="bar"}`,
			Type: &commonv1.ProfileType{
				ID:         "memory:inuse_space:bytes:space:bytes",
				Name:       "memory",
				SampleType: "inuse_space",
				SampleUnit: "bytes",
				PeriodType: "space",
				PeriodUnit: "bytes",
			},
			Start: int64(model.Earliest),
			End:   int64(model.Latest),
		}))
		require.NoError(b, err)
	}
}
