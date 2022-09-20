package firedb

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	googlev1 "github.com/grafana/fire/pkg/gen/google/v1"
	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/iter"
	firemodel "github.com/grafana/fire/pkg/model"
	"github.com/grafana/fire/pkg/testhelper"
)

func TestCreateLocalDir(t *testing.T) {
	dataPath := t.TempDir()
	localFile := dataPath + "/local"
	require.NoError(t, ioutil.WriteFile(localFile, []byte("d"), 0o644))
	_, err := New(&Config{
		DataPath:      dataPath,
		BlockDuration: 30 * time.Minute,
	}, log.NewNopLogger(), nil)
	require.Error(t, err)
	require.NoError(t, os.Remove(localFile))
	_, err = New(&Config{
		DataPath:      dataPath,
		BlockDuration: 30 * time.Minute,
	}, log.NewNopLogger(), nil)
	require.NoError(t, err)
}

var cpuProfileGenerator = func(tsNano int64, t testing.TB) (*googlev1.Profile, string) {
	p := parseProfile(t, "testdata/profile")
	p.TimeNanos = tsNano
	return p, "process_cpu"
}

func ingestProfiles(b testing.TB, db *FireDB, generator func(tsNano int64, t testing.TB) (*googlev1.Profile, string), from, to int64, step time.Duration, externalLabels ...*commonv1.LabelPair) {
	b.Helper()
	for i := from; i <= to; i += int64(step) {
		p, name := generator(i, b)
		require.NoError(b, db.Head().Ingest(
			context.Background(), p, uuid.New(), append(externalLabels, &commonv1.LabelPair{Name: model.MetricNameLabel, Value: name})...))
	}
}

type fakeBidiServerMergeProfilesStacktraces struct {
	profilesSent []*ingestv1.ProfileSets
	keep         [][]bool
	t            *testing.T
}

func (f *fakeBidiServerMergeProfilesStacktraces) Send(resp *ingestv1.MergeProfilesStacktracesResponse) error {
	f.profilesSent = append(f.profilesSent, testhelper.CloneProto(f.t, resp.SelectedProfiles))
	return nil
}

func (f *fakeBidiServerMergeProfilesStacktraces) Receive() (*ingestv1.MergeProfilesStacktracesRequest, error) {
	res := &ingestv1.MergeProfilesStacktracesRequest{
		Profiles: f.keep[0],
	}
	f.keep = f.keep[1:]
	return res, nil
}

func TestFilterProfiles(t *testing.T) {
	ctx := context.Background()
	profiles := lo.Times(11, func(i int) Profile {
		return ProfileWithLabels{
			Profile: &schemav1.Profile{TimeNanos: int64(i * int(time.Minute))},
			lbs:     firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)),
			fp:      model.Fingerprint(firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)).Hash()),
		}
	})
	in := iter.NewSliceIterator(profiles)
	bidi := &fakeBidiServerMergeProfilesStacktraces{
		keep: [][]bool{{}, {true}, {true}},
		t:    t,
	}
	filtered, err := filterProfiles[
		BidiServerMerge[*ingestv1.MergeProfilesStacktracesResponse, *ingestv1.MergeProfilesStacktracesRequest],
		*ingestv1.MergeProfilesStacktracesResponse,
		*ingestv1.MergeProfilesStacktracesRequest](ctx, in, 5, bidi)
	require.NoError(t, err)
	require.Equal(t, 2, len(filtered))
	require.Equal(t, 3, len(bidi.profilesSent))
	testhelper.EqualProto(t, []*ingestv1.ProfileSets{
		{
			LabelsSets: lo.Times(5, func(i int) *commonv1.Labels {
				return &commonv1.Labels{Labels: firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i))}
			}),
			Profiles: lo.Times(5, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64(i * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
		{
			LabelsSets: lo.Times(5, func(i int) *commonv1.Labels {
				return &commonv1.Labels{Labels: firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i+5))}
			}),
			Profiles: lo.Times(5, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64((i + 5) * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
		{
			LabelsSets: lo.Times(1, func(i int) *commonv1.Labels {
				return &commonv1.Labels{Labels: firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i+10))}
			}),
			Profiles: lo.Times(1, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64((i + 10) * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
	}, bidi.profilesSent)

	require.Equal(t, []Profile{
		ProfileWithLabels{
			Profile: &schemav1.Profile{TimeNanos: int64(5 * int(time.Minute))},
			lbs:     firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)),
			fp:      model.Fingerprint(firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)).Hash()),
		},
		ProfileWithLabels{
			Profile: &schemav1.Profile{TimeNanos: int64(10 * int(time.Minute))},
			lbs:     firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)),
			fp:      model.Fingerprint(firemodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)).Hash()),
		},
	}, filtered)
}
