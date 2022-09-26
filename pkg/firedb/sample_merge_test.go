package firedb

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	ingesterv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/iter"
	"github.com/grafana/fire/pkg/objstore/providers/filesystem"
	pprofth "github.com/grafana/fire/pkg/pprof/testhelper"
	"github.com/grafana/fire/pkg/testhelper"
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
				p.ForStacktrace("my", "other").AddSamples(1)
				p.ForStacktrace("my", "other").AddSamples(3)
				p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
					p.ForStacktrace("my", "other").AddSamples(1)
					p.ForStacktrace("my", "other").AddSamples(3)
					p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
					p.ForStacktrace("my", "other").AddSamples(1, 2, 3, 4)
					p.ForStacktrace("my", "other").AddSamples(3, 2, 3, 4)
					p.ForStacktrace("my", "other", "stack").AddSamples(3, 3, 3, 3)
					ps = append(ps, p)
				}
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktrace("my", "other").AddSamples(1)
					p.ForStacktrace("my", "other").AddSamples(3)
					p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
			db, err := New(Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, log.NewNopLogger(), nil)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			require.NoError(t, db.Flush(context.Background()))

			b, err := filesystem.NewBucket(filepath.Join(testPath, pathLocal))
			require.NoError(t, err)

			// open resulting block
			q := NewBlockQuerier(log.NewNopLogger(), b)
			require.NoError(t, q.Sync(context.Background()))

			profiles, err := q.queriers[0].SelectMatchingProfiles(ctx, &ingesterv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &commonv1.ProfileType{
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
				p.ForStacktrace("my", "other").AddSamples(1)
				p.ForStacktrace("my", "other").AddSamples(3)
				p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
					p.ForStacktrace("my", "other").AddSamples(1)
					p.ForStacktrace("my", "other").AddSamples(3)
					p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
					p.ForStacktrace("my", "other").AddSamples(1, 2, 3, 4)
					p.ForStacktrace("my", "other").AddSamples(3, 2, 3, 4)
					p.ForStacktrace("my", "other", "stack").AddSamples(3, 3, 3, 3)
					ps = append(ps, p)
				}
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktrace("my", "other").AddSamples(1)
					p.ForStacktrace("my", "other").AddSamples(3)
					p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
			db, err := New(Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, log.NewNopLogger(), nil)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}
			profiles, err := db.head.SelectMatchingProfiles(ctx, &ingesterv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &commonv1.ProfileType{
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
			stacktraces, err := db.head.MergeByStacktraces(ctx, profiles)
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
		expected []*commonv1.Series
		by       []string
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktrace("my", "other").AddSamples(1)
				p.ForStacktrace("my", "other").AddSamples(3)
				p.ForStacktrace("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				return
			},
			expected: []*commonv1.Series{
				{
					Labels: []*commonv1.LabelPair{},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 7}},
				},
			},
		},
		{
			name: "multiple profiles",
			by:   []string{"foo"},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*commonv1.Series{
				{
					Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
				{
					Labels: []*commonv1.LabelPair{{Name: "foo", Value: "buzz"}},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 1}},
				},
			},
		},
		{
			name: "multiple profile no by",
			by:   []string{},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*commonv1.Series{
				{
					Labels: []*commonv1.LabelPair{},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testPath := t.TempDir()
			db, err := New(Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, log.NewNopLogger(), nil)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			require.NoError(t, db.Flush(context.Background()))

			b, err := filesystem.NewBucket(filepath.Join(testPath, pathLocal))
			require.NoError(t, err)

			// open resulting block
			q := NewBlockQuerier(log.NewNopLogger(), b)
			require.NoError(t, q.Sync(context.Background()))

			profileIt, err := q.queriers[0].SelectMatchingProfiles(ctx, &ingesterv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &commonv1.ProfileType{
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
		expected []*commonv1.Series
		by       []string
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktrace("my", "other").AddSamples(1)
				p.ForStacktrace("my", "other").AddSamples(3)
				p.ForStacktrace("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				return
			},
			expected: []*commonv1.Series{
				{
					Labels: []*commonv1.LabelPair{},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 7}},
				},
			},
		},
		{
			name: "multiple profiles",
			by:   []string{"foo"},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*commonv1.Series{
				{
					Labels: []*commonv1.LabelPair{{Name: "foo", Value: "bar"}},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
				{
					Labels: []*commonv1.LabelPair{{Name: "foo", Value: "buzz"}},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 1}},
				},
			},
		},
		{
			name: "multiple profile no by",
			by:   []string{},
			in: func() (ps []*pprofth.ProfileBuilder) {
				p := pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(15*time.Second)).CPUProfile().WithLabels("foo", "buzz")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)

				p = pprofth.NewProfileBuilder(int64(30*time.Second)).CPUProfile().WithLabels("foo", "bar")
				p.ForStacktrace("my", "other").AddSamples(1)
				ps = append(ps, p)
				return
			},
			expected: []*commonv1.Series{
				{
					Labels: []*commonv1.LabelPair{},
					Points: []*commonv1.Point{{Timestamp: 15000, Value: 1}, {Timestamp: 15000, Value: 1}, {Timestamp: 30000, Value: 1}},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testPath := t.TempDir()
			db, err := New(Config{
				DataPath:         testPath,
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, log.NewNopLogger(), nil)
			require.NoError(t, err)
			ctx := context.Background()

			for _, p := range tc.in() {
				require.NoError(t, db.Head().Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			profileIt, err := db.Head().SelectMatchingProfiles(ctx, &ingesterv1.SelectProfilesRequest{
				LabelSelector: `{}`,
				Type: &commonv1.ProfileType{
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
			series, err := db.Head().MergeByLabels(ctx, iter.NewSliceIterator(profiles), tc.by...)
			require.NoError(t, err)

			testhelper.EqualProto(t, tc.expected, series)
		})
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
// func newSingleBlockQuerier(logger log.Logger, bucketReader fireobjstore.BucketReader, path string) (*singleBlockQuerier, error) {
// 	meta, _, err := block.MetaFromDir(path)
// 	if err != nil {
// 		return nil, err
// 	}
// 	q := &singleBlockQuerier{
// 		logger:       logger,
// 		bucketReader: fireobjstore.BucketReaderWithPrefix(bucketReader, meta.ULID.String()),
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
// 				p.ForStacktrace("my", "other").AddSamples(1)
// 				p.ForStacktrace("my", "other").AddSamples(3)
// 				p.ForStacktrace("my", "other", "stack").AddSamples(3)
// 				ps = append(ps, p)
// 				return
// 			},
// 			expected: []ProfileValue{
// 				{
// 					Profile: Profile{
// 						Labels:    firemodel.LabelsFromStrings("job", "foo", "instance", "bar"),
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
// 					p.ForStacktrace("my", "other").AddSamples(1)
// 					p.ForStacktrace("my", "other").AddSamples(3)
// 					p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
// 					p.ForStacktrace("my", "other").AddSamples(1, 2, 3, 4)
// 					p.ForStacktrace("my", "other").AddSamples(3, 2, 3, 4)
// 					p.ForStacktrace("my", "other", "stack").AddSamples(3, 3, 3, 3)
// 					ps = append(ps, p)
// 				}
// 				for i := 0; i < 3000; i++ {
// 					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
// 						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
// 					p.ForStacktrace("my", "other").AddSamples(1)
// 					p.ForStacktrace("my", "other").AddSamples(3)
// 					p.ForStacktrace("my", "other", "stack").AddSamples(3)
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
// 				Type: &commonv1.ProfileType{
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
// 				Labels:    firemodel.LabelsFromStrings("job", "foo", "series", fmt.Sprintf("%d", i)),
// 				Timestamp: model.TimeFromUnixNano(int64(15 * time.Second)),
// 			},
// 			Value: value,
// 		})
// 	}
// 	// profiles are store by labels then timestamp.
// 	sort.Slice(result, func(i, j int) bool {
// 		return firemodel.CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
// 	})
// 	return
// }
