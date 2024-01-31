package phlaredb

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/pprof"
	pprofth "github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func TestMergeSampleByStacktraces(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   func() ([]*pprofth.ProfileBuilder, *phlaremodel.Tree)
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder, tree *phlaremodel.Tree) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktraceString("my", "other").AddSamples(1)
				p.ForStacktraceString("my", "other").AddSamples(3)
				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				tree = new(phlaremodel.Tree)
				tree.InsertStack(4, "other", "my")
				tree.InsertStack(3, "stack", "other", "my")
				return ps, tree
			},
		},
		{
			name: "multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder, tree *phlaremodel.Tree) {
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1)
					p.ForStacktraceString("my", "other").AddSamples(3)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
					ps = append(ps, p)
				}
				tree = new(phlaremodel.Tree)
				tree.InsertStack(12000, "other", "my")
				tree.InsertStack(9000, "stack", "other", "my")
				return ps, tree
			},
		},
		{
			name: "filtering multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder, tree *phlaremodel.Tree) {
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
				tree = new(phlaremodel.Tree)
				tree.InsertStack(12000, "other", "my")
				tree.InsertStack(9000, "stack", "other", "my")
				return ps, tree
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := testContext(t)
			db, err := New(ctx, Config{
				DataPath:         contextDataDir(ctx),
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit, ctx.localBucketClient)
			require.NoError(t, err)

			input, expected := tc.in()
			for _, p := range input {
				require.NoError(t, db.Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			require.NoError(t, db.Flush(context.Background(), true, ""))

			b, err := filesystem.NewBucket(filepath.Join(contextDataDir(ctx), PathLocal))
			require.NoError(t, err)

			// open resulting block
			q := NewBlockQuerier(ctx, b)
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

			r, err := q.queriers[0].MergeByStacktraces(ctx, profiles)
			require.NoError(t, err)
			require.Equal(t, expected.String(), r.String())
		})
	}
}

func TestHeadMergeSampleByStacktraces(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   func() ([]*pprofth.ProfileBuilder, *phlaremodel.Tree)
	}{
		{
			name: "single profile",
			in: func() (ps []*pprofth.ProfileBuilder, tree *phlaremodel.Tree) {
				p := pprofth.NewProfileBuilder(int64(15 * time.Second)).CPUProfile()
				p.ForStacktraceString("my", "other").AddSamples(1)
				p.ForStacktraceString("my", "other").AddSamples(3)
				p.ForStacktraceString("my", "other", "stack").AddSamples(3)
				ps = append(ps, p)
				tree = new(phlaremodel.Tree)
				tree.InsertStack(4, "other", "my")
				tree.InsertStack(3, "stack", "other", "my")
				return ps, tree
			},
		},
		{
			name: "multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder, tree *phlaremodel.Tree) {
				for i := 0; i < 3000; i++ {
					p := pprofth.NewProfileBuilder(int64(15*time.Second)).
						CPUProfile().WithLabels("series", fmt.Sprintf("%d", i))
					p.ForStacktraceString("my", "other").AddSamples(1)
					p.ForStacktraceString("my", "other").AddSamples(3)
					p.ForStacktraceString("my", "other", "stack").AddSamples(3)
					ps = append(ps, p)
				}
				tree = new(phlaremodel.Tree)
				tree.InsertStack(12000, "other", "my")
				tree.InsertStack(9000, "stack", "other", "my")
				return ps, tree
			},
		},
		{
			name: "filtering multiple profiles",
			in: func() (ps []*pprofth.ProfileBuilder, tree *phlaremodel.Tree) {
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
				tree = new(phlaremodel.Tree)
				tree.InsertStack(12000, "other", "my")
				tree.InsertStack(9000, "stack", "other", "my")
				return ps, tree
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := testContext(t)
			db, err := New(ctx, Config{
				DataPath:         contextDataDir(ctx),
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit, ctx.localBucketClient)
			require.NoError(t, err)

			input, expected := tc.in()
			for _, p := range input {
				require.NoError(t, db.Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}
			profiles, err := db.queriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
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
			r, err := db.queriers()[0].MergeByStacktraces(ctx, profiles)
			require.NoError(t, err)
			require.Equal(t, expected.String(), r.String())
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
			ctx := testContext(t)
			db, err := New(ctx, Config{
				DataPath:         contextDataDir(ctx),
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit, ctx.localBucketClient)
			require.NoError(t, err)

			for _, p := range tc.in() {
				require.NoError(t, db.Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			require.NoError(t, db.Flush(context.Background(), true, ""))

			b, err := filesystem.NewBucket(filepath.Join(contextDataDir(ctx), PathLocal))
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
			series, err := q.queriers[0].MergeByLabels(ctx, iter.NewSliceIterator(profiles), nil, tc.by...)
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
			ctx := testContext(t)
			db, err := New(ctx, Config{
				DataPath:         contextDataDir(ctx),
				MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
			}, NoLimit, ctx.localBucketClient)
			require.NoError(t, err)

			for _, p := range tc.in() {
				require.NoError(t, db.Ingest(ctx, p.Profile, p.UUID, p.Labels...))
			}

			profileIt, err := db.queriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
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

			db.headQueriers()[0].Sort(profiles)
			series, err := db.headQueriers()[0].MergeByLabels(ctx, iter.NewSliceIterator(profiles), nil, tc.by...)
			require.NoError(t, err)

			testhelper.EqualProto(t, tc.expected, series)
		})
	}
}

func TestMergePprof(t *testing.T) {
	ctx := testContext(t)
	db, err := New(ctx, Config{
		DataPath:         contextDataDir(ctx),
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Ingest(ctx, generateProfile(t, i*1000), uuid.New(), &typesv1.LabelPair{
			Name:  model.MetricNameLabel,
			Value: "process_cpu",
		}))
	}

	require.NoError(t, db.Flush(context.Background(), true, ""))

	b, err := filesystem.NewBucket(filepath.Join(contextDataDir(ctx), PathLocal))
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
	result, err := q.queriers[0].MergePprof(ctx, iter.NewSliceIterator(profiles), 0, nil)
	require.NoError(t, err)

	data, err := proto.Marshal(generateProfile(t, 1))
	require.NoError(t, err)
	expected, err := profile.ParseUncompressed(data)
	require.NoError(t, err)
	for _, sample := range expected.Sample {
		sample.Value = []int64{sample.Value[0] * 3}
	}
	data, err = proto.Marshal(result)
	require.NoError(t, err)
	actual, err := profile.ParseUncompressed(data)
	require.NoError(t, err)
	compareProfile(t, expected.Compact(), actual.Compact())
}

func TestHeadMergePprof(t *testing.T) {
	ctx := testContext(t)
	db, err := New(ctx, Config{
		DataPath:         contextDataDir(ctx),
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Ingest(ctx, generateProfile(t, i*1000), uuid.New(), &typesv1.LabelPair{
			Name:  model.MetricNameLabel,
			Value: "process_cpu",
		}))
	}

	profileIt, err := db.queriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
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

	db.headQueriers()[0].Sort(profiles)
	result, err := db.headQueriers()[0].MergePprof(ctx, iter.NewSliceIterator(profiles), 0, nil)
	require.NoError(t, err)

	data, err := proto.Marshal(generateProfile(t, 1))
	require.NoError(t, err)
	expected, err := profile.ParseUncompressed(data)
	require.NoError(t, err)
	for _, sample := range expected.Sample {
		sample.Value = []int64{sample.Value[0] * 3}
	}
	data, err = proto.Marshal(result)
	require.NoError(t, err)
	actual, err := profile.ParseUncompressed(data)
	require.NoError(t, err)
	compareProfile(t, expected.Compact(), actual.Compact())
}

func TestMergeSpans(t *testing.T) {
	ctx := testContext(t)
	db, err := New(ctx, Config{
		DataPath:         contextDataDir(ctx),
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)

	require.NoError(t, db.Ingest(ctx, generateProfileWithSpans(t, 1000), uuid.New(), &typesv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: "process_cpu",
	}))

	require.NoError(t, db.Flush(context.Background(), true, ""))

	b, err := filesystem.NewBucket(filepath.Join(contextDataDir(ctx), PathLocal))
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
	spanSelector, err := phlaremodel.NewSpanSelector([]string{"badbadbadbadbadb"})
	require.NoError(t, err)
	result, err := q.queriers[0].MergeBySpans(ctx, iter.NewSliceIterator(profiles), spanSelector)
	require.NoError(t, err)

	expected := new(phlaremodel.Tree)
	expected.InsertStack(1, "bar", "foo")
	expected.InsertStack(2, "foo")

	require.Equal(t, expected.String(), result.String())
}

func TestHeadMergeSpans(t *testing.T) {
	ctx := testContext(t)
	db, err := New(ctx, Config{
		DataPath:         contextDataDir(ctx),
		MaxBlockDuration: time.Duration(100000) * time.Minute, // we will manually flush
	}, NoLimit, ctx.localBucketClient)
	require.NoError(t, err)

	require.NoError(t, db.Ingest(ctx, generateProfileWithSpans(t, 1000), uuid.New(), &typesv1.LabelPair{
		Name:  model.MetricNameLabel,
		Value: "process_cpu",
	}))

	profileIt, err := db.headQueriers().SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
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

	db.headQueriers()[0].Sort(profiles)
	spanSelector, err := phlaremodel.NewSpanSelector([]string{"badbadbadbadbadb"})
	require.NoError(t, err)

	result, err := db.headQueriers()[0].MergeBySpans(ctx, iter.NewSliceIterator(profiles), spanSelector)
	require.NoError(t, err)

	expected := new(phlaremodel.Tree)
	expected.InsertStack(1, "bar", "foo")
	expected.InsertStack(2, "foo")

	require.Equal(t, expected.String(), result.String())
}

func generateProfile(t *testing.T, ts int) *googlev1.Profile {
	t.Helper()

	prof, err := pprof.FromProfile(pprofth.FooBarProfile)

	require.NoError(t, err)
	prof.TimeNanos = int64(ts)
	return prof
}

func generateProfileWithSpans(t *testing.T, ts int) *googlev1.Profile {
	t.Helper()

	prof, err := pprof.FromProfile(pprofth.FooBarProfileWithSpans)

	require.NoError(t, err)
	prof.TimeNanos = int64(ts)
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
