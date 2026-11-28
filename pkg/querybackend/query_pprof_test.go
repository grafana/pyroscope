package querybackend

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
)

func TestResolvedPprofAggregation(t *testing.T) {
	encoded, err := os.ReadFile("../pprof/testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var source profilev1.Profile
	require.NoError(t, pprof.Unmarshal(encoded, &source))
	query := &queryv1.PprofQuery{MaxNodes: 131072, ProfileIdSelector: []string{"profile-id"}}
	for _, sanitize := range []bool{false, true} {
		for _, mode := range []string{"direct", "encoded-first", "direct-first", "concurrent"} {
			t.Run(fmt.Sprintf("sanitize=%t/%s", sanitize, mode), func(t *testing.T) {
				ra := newAggregator(&queryv1.InvokeRequest{Options: &queryv1.InvokeOptions{SanitizeOnMerge: sanitize}})
				var expected pprof.ProfileMerge
				const count = 4
				for range count {
					require.NoError(t, expected.Merge(source.CloneVT(), sanitize))
				}
				var g errgroup.Group
				for i := range count {
					merge := func() error {
						if mode == "encoded-first" && i == 0 || mode == "direct-first" && i > 0 || mode == "concurrent" && i%2 == 0 {
							return ra.aggregateReport(&queryv1.Report{ReportType: queryv1.ReportType_REPORT_PPROF,
								Pprof: &queryv1.PprofReport{Query: query.CloneVT(), Pprof: encoded}})
						}
						return ra.aggregateResolvedPprof(query, source.CloneVT())
					}
					if mode == "concurrent" {
						g.Go(merge)
					} else {
						require.NoError(t, merge())
					}
				}
				require.NoError(t, g.Wait())
				response := ra.response()
				require.Len(t, response.Reports, 1)
				report := response.Reports[0]
				require.Equal(t, queryv1.ReportType_REPORT_PPROF, report.ReportType)
				testhelper.EqualProto(t, query, report.Pprof.Query)
				var got profilev1.Profile
				require.NoError(t, pprof.Unmarshal(report.Pprof.Pprof, &got))
				testhelper.EqualProto(t, expected.Profile(), &got)
			})
		}
	}
}

func TestResolvedPprofEmptyAndSingleton(t *testing.T) {
	encoded, err := os.ReadFile("../pprof/testdata/go.cpu.labels.pprof")
	require.NoError(t, err)
	var source profilev1.Profile
	require.NoError(t, pprof.Unmarshal(encoded, &source))
	for _, input := range []*profilev1.Profile{nil, {}, &source} {
		ra := newAggregator(&queryv1.InvokeRequest{})
		query := &queryv1.PprofQuery{MaxNodes: 16384}
		require.NoError(t, ra.aggregateResolvedPprof(query, input.CloneVT()))
		var expected pprof.ProfileMerge
		require.NoError(t, expected.Merge(input.CloneVT(), false))
		response := ra.response()
		require.Len(t, response.Reports, 1)
		testhelper.EqualProto(t, query, response.Reports[0].Pprof.Query)
		var got profilev1.Profile
		require.NoError(t, pprof.Unmarshal(response.Reports[0].Pprof.Pprof, &got))
		testhelper.EqualProto(t, expected.Profile(), &got)
	}
}
