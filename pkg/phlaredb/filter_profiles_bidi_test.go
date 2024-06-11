package phlaredb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func TestFilterProfiles(t *testing.T) {
	ctx := context.Background()
	profiles := lo.Times(11, func(i int) Profile {
		return ProfileWithLabels{
			profile: &schemav1.InMemoryProfile{TimeNanos: int64(i * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)).Hash()),
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
		*ingestv1.MergeProfilesStacktracesRequest](ctx, []iter.Iterator[Profile]{in}, 5, bidi)
	require.NoError(t, err)
	require.Equal(t, 2, len(filtered[0]))
	require.Equal(t, 3, len(bidi.profilesSent))

	expectedSent := []*ingestv1.ProfileSets{
		{
			Fingerprints: lo.Times(5, func(i int) uint64 {
				return phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i)).Hash()
			}),
			Profiles: lo.Times(5, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64(i * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
		{
			Fingerprints: lo.Times(5, func(i int) uint64 {
				return phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i+5)).Hash()
			}),
			Profiles: lo.Times(5, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64((i + 5) * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
		{
			Fingerprints: lo.Times(1, func(i int) uint64 {
				return phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", i+10)).Hash()
			}),
			Profiles: lo.Times(1, func(i int) *ingestv1.SeriesProfile {
				return &ingestv1.SeriesProfile{Timestamp: int64(model.TimeFromUnixNano(int64((i + 10) * int(time.Minute)))), LabelIndex: int32(i)}
			}),
		},
	}
	testhelper.EqualProto(t, expectedSent, bidi.profilesSent)

	expectedFiltered := []Profile{
		ProfileWithLabels{
			profile: &schemav1.InMemoryProfile{TimeNanos: int64(5 * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 5)).Hash()),
		},
		ProfileWithLabels{
			profile: &schemav1.InMemoryProfile{TimeNanos: int64(10 * int(time.Minute))},
			lbs:     phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)),
			fp:      model.Fingerprint(phlaremodel.LabelsFromStrings("foo", "bar", "i", fmt.Sprintf("%d", 10)).Hash()),
		},
	}
	require.Equal(t, expectedFiltered, filtered[0])
}
