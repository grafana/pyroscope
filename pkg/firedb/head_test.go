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
	"github.com/samber/lo"
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

func TestHeadProfileTypes(t *testing.T) {
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New(), &commonv1.LabelPair{Name: "__name__", Value: "foo"}, &commonv1.LabelPair{Name: "job", Value: "foo"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New(), &commonv1.LabelPair{Name: "__name__", Value: "bar"}, &commonv1.LabelPair{Name: "namespace", Value: "fire"}))

	res, err := head.ProfileTypes(context.Background(), connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
	require.NoError(t, err)
	require.Equal(t, []string{"bar:type:unit:type:unit", "foo:type:unit:type:unit"}, res.Msg.Names)
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

	// compare the first profile deep
	profileJSON, err := json.Marshal(&resp.Msg.Profiles[0])
	require.NoError(t, err)
	require.JSONEq(t, `{
  "type": {
    "name": "memory",
    "sampleType": "type",
    "sampleUnit": "unit",
    "periodType": "type",
    "periodUnit": "unit"
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

func TestIngestDiff(t *testing.T) {
	ctx := context.Background()
	head, err := NewHead(t.TempDir())
	require.NoError(t, err)

	profile := newMemoryProfileBuilder(1)
	profile.ForStacktrace("a", "b", "c").AddMemorySample(1, 10, 1, 1)
	profile.ForStacktrace("a", "b", "c", "d").AddMemorySample(1, 10, 1, 1)

	err = head.Ingest(ctx, profile.Profile, profile.UUID, profile.labels...)
	require.NoError(t, err)

	profile = newMemoryProfileBuilder(2)
	profile.ForStacktrace("a", "b", "c").AddMemorySample(2, 20, 2, 2)
	profile.ForStacktrace("a", "b", "c", "d").AddMemorySample(3, 30, 2, 2)

	err = head.Ingest(ctx, profile.Profile, profile.UUID, profile.labels...)
	require.NoError(t, err)
}

type memoryProfileBuilder struct {
	*profilev1.Profile
	uuid.UUID
	labels []*commonv1.LabelPair
}

func newMemoryProfileBuilder(ts int64) *memoryProfileBuilder {
	return &memoryProfileBuilder{
		Profile: &profilev1.Profile{
			TimeNanos: ts,
			SampleType: []*profilev1.ValueType{
				{
					Unit: 4,
					Type: 3,
				},
				{
					Unit: 2,
					Type: 5,
				},
				{
					Unit: 4,
					Type: 6,
				},
				{
					Unit: 2,
					Type: 7,
				},
			},
			Mapping: []*profilev1.Mapping{
				{Id: 1, HasFunctions: true},
			},
			StringTable:       []string{"", "space", "bytes", "alloc_objects", "count", "alloc_space", "inuse_objects", "inuse_space"},
			DefaultSampleType: 5,
			PeriodType: &profilev1.ValueType{
				Unit: 2,
				Type: 1,
			},
		},
		UUID: uuid.New(),
		labels: []*commonv1.LabelPair{
			{
				Name:  "job",
				Value: "foo",
			},
			{
				Name:  model.MetricNameLabel,
				Value: "memory",
			},
		},
	}
}

func (m *memoryProfileBuilder) ForStacktrace(stacktraces ...string) *stacktraceBuilder {
	namePositions := lo.Map(stacktraces, func(stacktrace string, i int) int64 {
		for i, n := range m.StringTable {
			if n == stacktrace {
				return int64(i)
			}
		}
		m.StringTable = append(m.StringTable, stacktrace)
		return int64(len(m.StringTable) - 1)
	})

	// search functions
	functionIds := lo.Map(namePositions, func(namePos int64, i int) uint64 {
		for _, f := range m.Function {
			if f.Name == namePos {
				return f.Id
			}
		}
		f := &profilev1.Function{
			Name: namePos,
			Id:   uint64(len(m.Function)) + 1,
		}
		m.Function = append(m.Function, f)
		return f.Id
	})
	// search locations
	locationIDs := lo.Map(functionIds, func(functionId uint64, i int) uint64 {
		for _, l := range m.Location {
			if l.Line[0].FunctionId == functionId {
				return l.Id
			}
		}
		l := &profilev1.Location{
			MappingId: uint64(1),
			Line: []*profilev1.Line{
				{
					FunctionId: functionId,
				},
			},
			Id: uint64(len(m.Location)) + 1,
		}
		m.Location = append(m.Location, l)
		return l.Id
	})
	return &stacktraceBuilder{
		locationID:           locationIDs,
		memoryProfileBuilder: m,
	}
}

type stacktraceBuilder struct {
	locationID []uint64
	*memoryProfileBuilder
}

func (s *stacktraceBuilder) AddMemorySample(allocObjs int64, allocSpace int64, inuseObjs int64, inuseSpace int64) {
	s.Profile.Sample = append(s.Profile.Sample, &profilev1.Sample{
		LocationId: s.locationID,
		Value:      []int64{allocObjs, allocSpace, inuseObjs, inuseSpace},
	})
}
