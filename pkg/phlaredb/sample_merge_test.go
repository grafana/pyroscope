package phlaredb

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	"github.com/grafana/phlare/pkg/pprof"
	pprofth "github.com/grafana/phlare/pkg/pprof/testhelper"
	"github.com/grafana/phlare/pkg/testhelper"
)

func TestMergeSampleByStacktraces(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       func() []*pprofth.ProfileBuilder
		expected *ingestv1.MergeProfilesStacktracesResult
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktraceString("my", "other").AddSamples(1)
				p.ForStacktraceString("my", "other").AddSamples(3)
				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				return
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       4,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       3,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
		{
			name: "multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder) {
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1)
					p.ForStacktraceString("my", "other").AddSamples(3)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
					ps = append(ps, p)
				}
				return
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       12000,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       9000,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
		{
			name: "filtering multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder) {
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						MemoryProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1, 2, 3, 4)
					p.ForStacktraceString("my", "other").AddSamples(3, 2, 3, 4)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3, 3, 3, 3)
					ps = append(ps, p)
				}
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1)
					p.ForStacktraceString("my", "other").AddSamples(3)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
					ps = append(ps, p)
				}
				return
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       12000,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       9000,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testPath := t.TempDir()
			db, err := New(context.Background(), Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			require.NoError(t, db.Flush(context.Background()))

			b, err := filesystem.NewBucket(filepath.Join(testPath, pathLocal))
			require.NoError(t, err)

			// open resulting block
			q := NewBlockQuerier(context.Background(), b)
			require.NoError(t, q.Sync(context.Background()))

			profiles, err := q.queriers[0].SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &typesv1.ProfileType{
					Name:       "process_cpu",
					SampleType: "cpu",
					SampleUnit: "nanoseconds",
					PeriodType: "cpu",
					PeriodUnit: "nanoseconds",
				},
				Start: int64(model.TimeFromUnixNano(0)),
				End:   int64(model.TimeFromUnixNano(int64(1 * time.Minute))),
			})
			require.NoError(t, err)

			stacktraces, err := q.queriers[0].MergeByStacktraces(ctx, profiles)
			require.NoError(t, err)
			sort.Slice(tc.expected.Stacktraces, func(i, j int) bool {
				return len(tc.expected.Stacktraces[i].FunctionIds) < len(tc.expected.Stacktraces[j].FunctionIds)
			})
			sort.Slice(stacktraces.Stacktraces, func(i, j int) bool {
				return len(stacktraces.Stacktraces[i].FunctionIds) < len(stacktraces.Stacktraces[j].FunctionIds)
			})
			testhelper.EqualProto(t, tc.expected, stacktraces)
		})
	}
}

func TestHeadMergeSampleByStacktraces(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       func() []*pprofth.ProfileBuilder
		expected *ingestv1.MergeProfilesStacktracesResult
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktraceString("my", "other").AddSamples(1)
				p.ForStacktraceString("my", "other").AddSamples(3)
				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				return
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       4,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       3,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
		{
			name: "multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder) {
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1)
					p.ForStacktraceString("my", "other").AddSamples(3)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
					ps = append(ps, p)
				}
				return
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       12000,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       9000,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
		{
			name: "filtering multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder) {
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						MemoryProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1, 2, 3, 4)
					p.ForStacktraceString("my", "other").AddSamples(3, 2, 3, 4)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3, 3, 3, 3)
					ps = append(ps, p)
				}
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1)
					p.ForStacktraceString("my", "other").AddSamples(3)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
					ps = append(ps, p)
				}
				return
			},
			expected: &ingestv1.MergeProfilesStacktracesResult{
				Stacktraces: []*ingestv1.StacktraceSample{
					{
						FunctionIds: []int32{0, 1},
						Value:       12000,
					},
					{
						FunctionIds: []int32{0, 1, 2},
						Value:       9000,
					},
				},
				FunctionNames: []string{"my", "other", "stack"},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testPath := t.TempDir()
			db, err := New(context.Background(), Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}
			profiles, err := db.head.Queriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &typesv1.ProfileType{
					Name:       "process_cpu",
					SampleType: "cpu",
					SampleUnit: "nanoseconds",
					PeriodType: "cpu",
					PeriodUnit: "nanoseconds",
				},
				Start: int64(model.TimeFromUnixNano(0)),
				End:   int64(model.TimeFromUnixNano(int64(1 * time.Minute))),
			})
			require.NoError(t, err)
			stacktraces, err := db.head.Queriers()[0].MergeByStacktraces(ctx, profiles)
			require.NoError(t, err)

			sort.Slice(tc.expected.Stacktraces, func(i, j int) bool {
				return len(tc.expected.Stacktraces[i].FunctionIds) < len(tc.expected.Stacktraces[j].FunctionIds)
			})
			sort.Slice(stacktraces.Stacktraces, func(i, j int) bool {
				return len(stacktraces.Stacktraces[i].FunctionIds) < len(stacktraces.Stacktraces[j].FunctionIds)
			})
			testhelper.EqualProto(t, tc.expected, stacktraces)
		})
	}
}

func TestMergeSampleByLabels(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       func() []*pprofth.ProfileBuilder
		expected []*typesv1.Series
		by       []string
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktraceString("my", "other").AddSamples(1)
				p.ForStacktraceString("my", "other").AddSamples(3)
				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				return
			},
			expected: []*typesv1.Series{
				{
					Labels: []*typesv1.LabelPair{},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 7}},
				},
			},
		},
		{
			name: "multiple profiles",
			by:   []string{"foo"},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*typesv1.Series{
				{
					Labels: []*typesv1.LabelPair{{Name: "foo", Value: "bar"}},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
				{
					Labels: []*typesv1.LabelPair{{Name: "foo", Value: "buzz"}},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 1}},
				},
			},
		},
		{
			name: "multiple profile no by",
			by:   []string{},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*typesv1.Series{
				{
					Labels: []*typesv1.LabelPair{},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testPath := t.TempDir()
			db, err := New(context.Background(), Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			require.NoError(t, db.Flush(context.Background()))

			b, err := filesystem.NewBucket(filepath.Join(testPath, pathLocal))
			require.NoError(t, err)

			// open resulting block
			q := NewBlockQuerier(context.Background(), b)
			require.NoError(t, q.Sync(context.Background()))

			profileIt, err := q.queriers[0].SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &typesv1.ProfileType{
					Name:       "process_cpu",
					SampleType: "cpu",
					SampleUnit: "nanoseconds",
					PeriodType: "cpu",
					PeriodUnit: "nanoseconds",
				},
				Start: int64(model.TimeFromUnixNano(0)),
				End:   int64(model.TimeFromUnixNano(int64(1 * time.Minute))),
			})
			require.NoError(t, err)
			profiles, err := iter.Slice(profileIt)
			require.NoError(t, err)

			q.queriers[0].Sort(profiles)
			series, err := q.queriers[0].MergeByLabels(ctx, iter.NewSliceIterator(profiles), tc.by...)
			require.NoError(t, err)

			testhelper.EqualProto(t, tc.expected, series)
		})
	}
}

func TestHeadMergeSampleByLabels(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       func() []*pprofth.ProfileBuilder
		expected []*typesv1.Series
		by       []string
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktraceString("my", "other").AddSamples(1)
				p.ForStacktraceString("my", "other").AddSamples(3)
				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				return
			},
			expected: []*typesv1.Series{
				{
					Labels: []*typesv1.LabelPair{},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 7}},
				},
			},
		},
		{
			name: "multiple profiles",
			by:   []string{"foo"},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*typesv1.Series{
				{
					Labels: []*typesv1.LabelPair{{Name: "foo", Value: "bar"}},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
				{
					Labels: []*typesv1.LabelPair{{Name: "foo", Value: "buzz"}},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 1}},
				},
			},
		},
		{
			name: "multiple profile no by",
			by:   []string{},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktraceString("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*typesv1.Series{
				{
					Labels: []*typesv1.LabelPair{},
					Points: []*typesv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testPath := t.TempDir()
			db, err := New(context.Background(), Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			profileIt, err := db.Head().Queriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &typesv1.ProfileType{
					Name:       "process_cpu",
					SampleType: "cpu",
					SampleUnit: "nanoseconds",
					PeriodType: "cpu",
					PeriodUnit: "nanoseconds",
				},
				Start: int64(model.TimeFromUnixNano(0)),
				End:   int64(model.TimeFromUnixNano(int64(1 * time.Minute))),
			})
			require.NoError(t, err)
			profiles, err := iter.Slice(profileIt)
			require.NoError(t, err)

			db.Head().Sort(profiles)
			series, err := db.Head().Queriers()[0].MergeByLabels(ctx, iter.NewSliceIterator(profiles), tc.by...)
			require.NoError(t, err)

			testhelper.EqualProto(t, tc.expected, series)
		})
	}
}

func TestMergePprof(t *testing.T) {
	testPath := t.TempDir()
	db, err := New(context.Background(), Config{
		DataPath:         testPath,
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit)
	require.NoError(t, err)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Head().Ingest(ctx, generateProfile(t), uuid.New(), &typesv1.LabelPair{
			Name:  model.MetricNameLabel,
			Value: "process_cpu",
		}))
	}

	require.NoError(t, db.Flush(context.Background()))

	b, err := filesystem.NewBucket(filepath.Join(testPath, pathLocal))
	require.NoError(t, err)

	// open resulting block
	q := NewBlockQuerier(context.Background(), b)
	require.NoError(t, q.Sync(context.Background()))

	profileIt, err := q.queriers[0].SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
		LabelSelector: `{}`,
		Type: &typesv1.ProfileType{
			Name:       "process_cpu",
			SampleType: "cpu",
			SampleUnit: "nanoseconds",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		Start: int64(model.TimeFromUnixNano(0)),
		End:   int64(model.TimeFromUnixNano(int64(1 * time.Minute))),
	})
	require.NoError(t, err)
	profiles, err := iter.Slice(profileIt)
	require.NoError(t, err)

	q.queriers[0].Sort(profiles)
	result, err := q.queriers[0].MergePprof(ctx, iter.NewSliceIterator(profiles))
	require.NoError(t, err)

	data, err := proto.Marshal(generateProfile(t))
	require.NoError(t, err)
	expected, err := profile.ParseUncompressed(data)
	require.NoError(t, err)
	for _, sample := range expected.Sample {
		sample.Value = []int64{sample.Value[0] * 3}
	}
	compareProfile(t, expected, result)
}

func TestHeadMergePprof(t *testing.T) {
	testPath := t.TempDir()
	db, err := New(context.Background(), Config{
		DataPath:         testPath,
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit)
	require.NoError(t, err)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Head().Ingest(ctx, generateProfile(t), uuid.New(), &typesv1.LabelPair{
			Name:  model.MetricNameLabel,
			Value: "process_cpu",
		}))
	}

	profileIt, err := db.Head().Queriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
		LabelSelector: `{}`,
		Type: &typesv1.ProfileType{
			Name:       "process_cpu",
			SampleType: "cpu",
			SampleUnit: "nanoseconds",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		Start: int64(model.TimeFromUnixNano(0)),
		End:   int64(model.TimeFromUnixNano(int64(1 * time.Minute))),
	})
	require.NoError(t, err)
	profiles, err := iter.Slice(profileIt)
	require.NoError(t, err)

	db.Head().Sort(profiles)
	result, err := db.Head().Queriers()[0].MergePprof(ctx, iter.NewSliceIterator(profiles))
	require.NoError(t, err)

	data, err := proto.Marshal(generateProfile(t))
	require.NoError(t, err)
	expected, err := profile.ParseUncompressed(data)
	require.NoError(t, err)
	for _, sample := range expected.Sample {
		sample.Value = []int64{sample.Value[0] * 3}
	}
	compareProfile(t, expected, result)
}

func generateProfile(t *testing.T) *googlev1.Profile {
	t.Helper()

	prof, err := pprof.FromProfile(pprofth.FooBarProfile)

	require.NoError(t, err)
	return prof
}

func compareProfile(t *testing.T, expected, actual *profile.Profile) {
	t.Helper()
	compareProfileSlice(t, expected.Sample, actual.Sample)
	compareProfileSlice(t, expected.Mapping, actual.Mapping)
	compareProfileSlice(t, expected.Location, actual.Location)
	compareProfileSlice(t, expected.Function, actual.Function)
}

// compareProfileSlice compares two slices of profile data.
// It ignores ID, un-exported fields.
func compareProfileSlice[T any](t *testing.T, expected, actual []T) {
	t.Helper()
	lessMapping := func(a, b *profile.Mapping) bool { return a.BuildID < b.BuildID }
	lessSample := func(a, b *profile.Sample) bool {
		if len(a.Value) != len(b.Value) {
			return len(a.Value) < len(b.Value)
		}
		for i := range a.Value {
			if a.Value[i] != b.Value[i] {
				return a.Value[i] < b.Value[i]
			}
		}
		return false
	}
	lessLocation := func(a, b *profile.Location) bool { return a.Address < b.Address }
	lessFunction := func(a, b *profile.Function) bool { return a.Name < b.Name }

	if diff := cmp.Diff(expected, actual, cmpopts.IgnoreUnexported(
		profile.Mapping{}, profile.Function{}, profile.Line{}, profile.Location{}, profile.Sample{}, profile.ValueType{}, profile.Profile{},
	), cmpopts.SortSlices(lessMapping), cmpopts.SortSlices(lessSample), cmpopts.SortSlices(lessLocation), cmpopts.SortSlices(lessFunction),
		cmpopts.IgnoreFields(profile.Mapping{}, "ID"),
		cmpopts.IgnoreFields(profile.Location{}, "ID"),
		cmpopts.IgnoreFields(profile.Function{}, "ID"),
	); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

// func BenchmarkSelectBlockProfiles(b *testing.B) {
// 	fs, err := filesystem.NewBucket("./testdata/")
// 	require.NoError(b, err)

// 	q, err := newSingleBlockQuerier(log.NewLogfmtLogger(os.Stdout), fs, "./testdata/01GD0EKBP0DENYEFVS5SB0K9WG/")
// 	require.NoError(b, err)
// 	require.NoError(b, q.open(context.TODO()))

// 	stacktraceAggrValues := map[int64]*ingestv1.StacktraceSample{}
// 	for i := 0; i < 1000; i++ {
// 		stacktraceAggrValues[int64(i)] = &ingestv1.StacktraceSample{
// 			Value: 1,
// 		}
// 	}
// 	b.ResetTimer()
// 	b.ReportAllocs()

// 	for i := 0; i < b.N; i++ {
// 		require.NoError(b, err)
// 		_, err = q.resolveSymbols(context.Background(), stacktraceAggrValues)
// 		require.NoError(b, err)
// 	}
// }
// func newSingleBlockQuerier(logger log.Logger, bucketReader phlareobjstore.BucketReader, path string) (*singleBlockQuerier, error) {
// 	meta, _, err := block.MetaFromDir(path)
// 	if err != nil {
// 		return nil, err
// 	}
// 	q := &singleBlockQuerier{
// 		logger:       logger,
// 		bucketReader: phlareobjstore.BucketReaderWithPrefix(bucketReader, meta.ULID.String()),
// 		meta:         meta,
// 	}
// 	q.tables = []tableReader{
// 		&q.strings,
// 		&q.functions,
// 		&q.locations,
// 		&q.stacktraces,
// 		&q.profiles,
// 	}
// 	return q, nil
// }

// func TestMergeSampleByProfile(t *testing.T) {
// 	for _, tc := range []struct {
// 		name     string
// 		in       func() []*pprofth.ProfileBuilder
// 		expected []ProfileValue
// 	}{
// 		{
// 			name: "single profile",
// 			in: func() (ps []*pprofth.ProfileBuilder) {
// 				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().
// 					WithLabels("instance", "bar")
// 				p.ForStacktraceString("my", "other").AddSamples(1)
// 				p.ForStacktraceString("my", "other").AddSamples(3)
// 				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
// 				ps = append(ps, p)
// 				return
// 			},
// 			expected: []ProfileValue{
// 				{
// 					Profile: Profile{
// 						Labels:    phlaremodel.LabelsFromStrings("job", "foo", "instance", "bar"),
// 						Timestamp: model.TimeFromUnixNano(int64(15 * time.Second)),
// 					},
// 					Value: 7,
// 				},
// 			},
// 		},
// 		{
// 			name: "multiple profiles",
// 			in: func() (ps []*pprofth.ProfileBuilder) {
// 				for i := 0; i < 3000; i++ {
// 					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
// 						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
// 					p.ForStacktraceString("my", "other").AddSamples(1)
// 					p.ForStacktraceString("my", "other").AddSamples(3)
// 					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
// 					ps = append(ps, p)
// 				}
// 				return
// 			},
// 			expected: generateProfileValues(3000, 7),
// 		},
// 		{
// 			name: "filtering multiple profiles",
// 			in: func() (ps []*pprofth.ProfileBuilder) {
// 				for i := 0; i < 3000; i++ {
// 					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
// 						MemoryProfile().WithLabels("series", fmt.Sprintf("%d", i))
// 					p.ForStacktraceString("my", "other").AddSamples(1, 2, 3, 4)
// 					p.ForStacktraceString("my", "other").AddSamples(3, 2, 3, 4)
// 					p.ForStacktraceString("my", "other", "stack").AddSamples(3, 3, 3, 3)
// 					ps = append(ps, p)
// 				}
// 				for i := 0; i < 3000; i++ {
// 					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
// 						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
// 					p.ForStacktraceString("my", "other").AddSamples(1)
// 					p.ForStacktraceString("my", "other").AddSamples(3)
// 					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
// 					ps = append(ps, p)
// 				}
// 				return
// 			},
// 			expected: generateProfileValues(3000, 7),
// 		},
// 	} {
// 		tc := tc
// 		t.Run(tc.name, func(t *testing.T) {
// 			testPath := t.TempDir()
// 			db, err := New(&Config{
// 				DataPath:      testPath,
// 				BlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
// 			}, log.NewNopLogger(), nil)
// 			require.NoError(t, err)
// 			ctx := context.Background()

// 			for _, p := range tc.in() {
// 				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
// 			}

// 			require.NoError(t, db.Flush(context.Background()))

// 			b, err := filesystem.NewBucket(filepath.Join(testPath, pathLocal))
// 			require.NoError(t, err)

// 			// open resulting block
// 			q := NewBlockQuerier(log.NewNopLogger(), b)
// 			require.NoError(t, q.Sync(context.Background()))

// 			merger, err := q.queriers[0].SelectMerge(ctx, SelectMergeRequest{
// 				LabelSelector: `{}`,
// 				Type: &typesv1.ProfileType{
// 					Name:       "process_cpu",
// 					SampleType: "cpu",
// 					SampleUnit: "nanoseconds",
// 					PeriodType: "cpu",
// 					PeriodUnit: "nanoseconds",
// 				},
// 				Start: model.TimeFromUnixNano(0),
// 				End:   model.TimeFromUnixNano(int64(1 * time.Minute)),
// 			})
// 			require.NoError(t, err)
// 			profiles := merger.SelectedProfiles()
// 			it, err := merger.MergeByProfile(profiles)
// 			require.NoError(t, err)

// 			actual := []ProfileValue{}
// 			for it.Next() {
// 				val := it.At()
// 				val.Labels = val.Labels.WithoutPrivateLabels()
// 				actual = append(actual, val)
// 			}
// 			require.NoError(t, it.Err())
// 			require.NoError(t, it.Close())
// 			for i := range actual {
// 				actual[i].Profile.RowNum = 0
// 				actual[i].Profile.Fingerprint = 0
// 			}

// 			testhelper.EqualProto(t, tc.expected, actual)
// 		})
// 	}
// }

// func generateProfileValues(count int, value int64) (result []ProfileValue) {
// 	for i := 0; i < count; i++ {
// 		result = append(result, ProfileValue{
// 			Profile: Profile{
// 				Labels:    phlaremodel.LabelsFromStrings("job", "foo", "series", fmt.Sprintf("%d", i)),
// 				Timestamp: model.TimeFromUnixNano(int64(15 * time.Second)),
// 			},
// 			Value: value,
// 		})
// 	}
// 	// profiles are store by labels then timestamp.
// 	sort.Slice(result, func(i, j int) bool {
// 		return phlaremodel.CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
// 	})
// 	return
// }
