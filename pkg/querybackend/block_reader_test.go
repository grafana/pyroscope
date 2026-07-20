package querybackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

type testSuite struct {
	suite.Suite
	dir string

	ctx    context.Context
	logger *test.TestingLogger
	bucket *memory.InMemBucket
	blocks []*metastorev1.BlockMeta

	reader *BlockReader
	meta   []*metastorev1.BlockMeta
	plan   *queryv1.QueryPlan
	tenant []string
}

func (s *testSuite) SetupSuite() {
	s.bucket = memory.NewInMemBucket()
	s.loadFromDir(s.dir)
}

func (s *testSuite) SetupTest() {
	s.ctx = context.Background()
	s.logger = test.NewTestingLogger(s.T())
	s.reader = NewBlockReader(s.logger, &objstore.ReaderAtBucket{Bucket: s.bucket}, nil)
	s.reader.Overrides = validation.MockDefaultOverrides()
	s.meta = make([]*metastorev1.BlockMeta, len(s.blocks))
	for i, b := range s.blocks {
		s.meta[i] = b.CloneVT()
	}
	s.sanitizeMetadata()
	s.plan = queryplan.Build(s.meta, 10, 10)
	s.tenant = make([]string, 0)
	for _, b := range s.plan.Root.Blocks {
		for _, d := range b.Datasets {
			s.tenant = append(s.tenant, b.StringTable[d.Tenant])
		}
	}
}

func (s *testSuite) loadFromDir(dir string) {
	s.Require().NoError(filepath.WalkDir(dir, s.visitPath))
}

func (s *testSuite) visitPath(path string, e os.DirEntry, err error) error {
	if err != nil || e.IsDir() {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var md metastorev1.BlockMeta
	if err = metadata.Decode(b, &md); err != nil {
		return err
	}
	md.Size = uint64(len(b))
	s.blocks = append(s.blocks, &md)
	return s.bucket.Upload(context.Background(), block.ObjectPath(&md), bytes.NewReader(b))
}

func (s *testSuite) sanitizeMetadata() {
	// We read the whole block metadata, including all the datasets.
	// In practice, this is never the case – metadata queries either
	// return the datasets to be read or the dataset index.
	hasIndex := 0
	total := 0
	for _, m := range s.meta {
		for _, d := range m.Datasets {
			total++
			if block.DatasetFormat(d.Format) == block.DatasetFormat1 {
				m.Datasets = slices.DeleteFunc(m.Datasets, func(x *metastorev1.Dataset) bool {
					return x.Format == 0
				})
				hasIndex++
				break
			}
		}
	}
	// We ensure that there are both cases.
	s.Assert().NotZero(total)
	s.Assert().NotZero(hasIndex)
}

func (s *testSuite) BeforeTest(_, _ string) {}

func (s *testSuite) AfterTest(_, _ string) {}

func TestSuite(t *testing.T) { suite.Run(t, &testSuite{dir: "testdata/samples"}) }

func (s *testSuite) Test_QueryTree_All() {

	expected, err := os.ReadFile("testdata/fixtures/tree_16.txt")
	s.Require().NoError(err)

	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
	s.Require().NoError(err)

	s.Assert().Equal(string(expected), tree.String())
}

func (s *testSuite) Test_QueryTree_Filter() {
	expected, err := os.ReadFile("testdata/fixtures/tree_16_slow.txt")
	s.Require().NoError(err)

	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: `{service_name="test-app",function="slow"}`,
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
	s.Require().NoError(err)

	s.Assert().Equal(string(expected), tree.String())
}

func (s *testSuite) Test_QueryPprof_Metadata() {
	selector := `{service_name="test-app",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: selector,
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{},
		}},
		Tenant: s.tenant,
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)

	var p profilev1.Profile
	s.Require().NoError(pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &p))

	s.Assert().Len(p.SampleType, 1)
	s.Assert().Equal("cpu", p.StringTable[p.SampleType[0].Type])
	s.Assert().Equal("nanoseconds", p.StringTable[p.SampleType[0].Unit])

	s.Assert().NotNil(p.PeriodType)
	s.Assert().Equal("cpu", p.StringTable[p.PeriodType.Type])
	s.Assert().Equal("nanoseconds", p.StringTable[p.PeriodType.Unit])
}

func (s *testSuite) Test_DatasetIndex_SeriesLabels_GroupBy() {
	selector := `{service_repository="https://github.com/grafana/pyroscope"}`
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: selector,
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{
				LabelNames: []string{"service_name", "__profile_type__"},
			},
		}},
		Tenant: s.tenant,
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)

	expected, err := os.ReadFile("testdata/fixtures/series_labels_by.json")
	s.Require().NoError(err)
	actual, _ := json.Marshal(resp.Reports[0].SeriesLabels)
	s.Assert().JSONEq(string(expected), string(actual))
}

func (s *testSuite) Test_SeriesLabels() {
	selector := `{service_name="pyroscope"}`
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: selector,
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType:    queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{},
		}},
		Tenant: s.tenant,
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)

	expected, err := os.ReadFile("testdata/fixtures/series_labels.json")
	s.Require().NoError(err)
	actual, _ := json.Marshal(resp.Reports[0].SeriesLabels)
	s.Assert().JSONEq(string(expected), string(actual))
}

var startTime = time.Unix(1739263329, 0)

const (
	fixtureMatchingSpanID    = "0000000000000001"
	fixtureNonMatchingSpanID = "ffffffffffffffff"

	spanSelectorWantBaseline = "baseline"
	spanSelectorWantEmpty    = "empty"
	spanSelectorWantFiltered = "filtered"
)

func (s *testSuite) Test_QueryTimeSeries() {
	query := &queryv1.Query{
		QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
		TimeSeries: &queryv1.TimeSeriesQuery{
			GroupBy: []string{"service_name"},
			Step:    30.0,
		},
	}

	req := &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(time.Hour).UnixMilli(),
		Query:         []*queryv1.Query{query},
		QueryPlan:     s.plan,
		LabelSelector: "{}",
		Tenant:        s.tenant,
	}

	resp, err := s.reader.Invoke(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	s.Require().NotNil(resp.Reports[0].TimeSeries)

	actual, _ := json.Marshal(resp.Reports[0].TimeSeries.TimeSeries)
	expected, err := os.ReadFile("testdata/fixtures/time_series.json")
	s.Require().NoError(err)
	s.Assert().JSONEq(string(expected), string(actual))
}

// When there is only one report we don't run the aggregate method. This check ensures that the timeseries, is still correctly formatted.
func (s *testSuite) Test_QueryTimeSeriesOneReport() {
	query := &queryv1.Query{
		QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
		TimeSeries: &queryv1.TimeSeriesQuery{
			GroupBy: []string{"service_name"},
			Step:    30.0,
		},
	}

	// shorten plan so there is only one report
	shorterPlan := s.plan.CloneVT()
	shorterPlan.Root = s.plan.Root.CloneVT()
	shorterPlan.Root.Blocks = s.plan.Root.Blocks[:1]

	req := &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(time.Hour).UnixMilli(),
		Query:         []*queryv1.Query{query},
		QueryPlan:     shorterPlan,
		LabelSelector: "{}",
		Tenant:        s.tenant,
	}

	resp, err := s.reader.Invoke(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	s.Require().NotNil(resp.Reports[0].TimeSeries)

	actual, _ := json.Marshal(resp.Reports[0].TimeSeries.TimeSeries)
	expected, err := os.ReadFile("testdata/fixtures/time_series_first_block.json")
	s.Require().NoError(err)
	s.Assert().JSONEq(string(expected), string(actual))
}

func (s *testSuite) Test_QueryTree_All_Tenant_Isolation() {
	queryTenant := "some-tenant"

	s.Require().NotContains(s.tenant, queryTenant)

	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: []string{queryTenant},
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 0)
}

func (s *testSuite) Test_ProfileIDSelector() {
	// Get a real profile ID for valid test case
	validProfileID := s.getProfileIDFromExemplars(s.T())

	// Load baseline fixture for tree comparison
	baselineTree, err := os.ReadFile("testdata/fixtures/tree_16.txt")
	s.Require().NoError(err)

	// Get baseline tree for comparison
	allTreeResp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	allTree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](allTreeResp.Reports[0].Tree.Tree)
	s.Require().NoError(err)

	// Get baseline pprof for comparison
	allPprofResp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	var allProfile profilev1.Profile
	err = pprof.Unmarshal(allPprofResp.Reports[0].Pprof.Pprof, &allProfile)
	s.Require().NoError(err)

	tests := []struct {
		queryType         queryv1.QueryType
		name              string
		profileIDSelector []string
		wantErr           bool
		expectBaseline    bool
		expectFiltered    bool
		expectEmpty       bool
	}{
		// Tree query tests
		{queryv1.QueryType_QUERY_TREE, "tree/invalid UUID returns error", []string{"invalid-uuid"}, true, false, false, false},
		{queryv1.QueryType_QUERY_TREE, "tree/empty selector returns baseline", []string{}, false, true, false, false},
		{queryv1.QueryType_QUERY_TREE, "tree/nil selector returns baseline", nil, false, true, false, false},
		{queryv1.QueryType_QUERY_TREE, "tree/non-existent UUID returns empty result", []string{"00000000-0000-0000-0000-000000000000"}, false, false, false, true},
		{queryv1.QueryType_QUERY_TREE, "tree/valid UUID filters to single profile", []string{validProfileID}, false, false, true, false},

		// Pprof query tests
		{queryv1.QueryType_QUERY_PPROF, "pprof/invalid UUID returns error", []string{"not-a-uuid"}, true, false, false, false},
		{queryv1.QueryType_QUERY_PPROF, "pprof/empty selector returns baseline", []string{}, false, true, false, false},
		{queryv1.QueryType_QUERY_PPROF, "pprof/nil selector returns baseline", nil, false, true, false, false},
		{queryv1.QueryType_QUERY_PPROF, "pprof/non-existent UUID returns empty result", []string{"00000000-0000-0000-0000-000000000000"}, false, false, false, true},
		{queryv1.QueryType_QUERY_PPROF, "pprof/valid UUID filters to single profile", []string{validProfileID}, false, false, true, false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var query *queryv1.Query
			var reqStartTime, reqEndTime int64

			if tt.queryType == queryv1.QueryType_QUERY_TREE {
				reqEndTime = time.Now().UnixMilli()
				query = &queryv1.Query{
					QueryType: queryv1.QueryType_QUERY_TREE,
					Tree: &queryv1.TreeQuery{
						MaxNodes:          16,
						ProfileIdSelector: tt.profileIDSelector,
					},
				}
			} else {
				reqStartTime = startTime.UnixMilli()
				reqEndTime = startTime.Add(5 * time.Minute).UnixMilli()
				query = &queryv1.Query{
					QueryType: queryv1.QueryType_QUERY_PPROF,
					Pprof: &queryv1.PprofQuery{
						ProfileIdSelector: tt.profileIDSelector,
					},
				}
			}

			resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
				StartTime:     reqStartTime,
				EndTime:       reqEndTime,
				LabelSelector: "{}",
				QueryPlan:     s.plan,
				Query:         []*queryv1.Query{query},
				Tenant:        s.tenant,
			})

			if tt.wantErr {
				s.Require().Error(err)
				s.Require().Nil(resp)
				return
			}

			s.Require().NoError(err)
			s.Require().NotNil(resp)
			s.Require().Len(resp.Reports, 1)

			if tt.queryType == queryv1.QueryType_QUERY_TREE {
				tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
				s.Require().NoError(err)

				if tt.expectBaseline {
					s.Assert().Equal(string(baselineTree), tree.String())
				}
				if tt.expectEmpty {
					s.Assert().Zero(tree.Total())
				}
				if tt.expectFiltered {
					s.Assert().Less(tree.Total(), allTree.Total())
					s.Assert().NotZero(tree.Total())
				}
			} else {
				var profile profilev1.Profile
				err = pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &profile)
				s.Require().NoError(err)

				if tt.expectBaseline {
					s.Assert().Equal(len(allProfile.Sample), len(profile.Sample))
				}
				if tt.expectEmpty {
					s.Assert().Zero(len(profile.Sample))
				}
				if tt.expectFiltered {
					s.Assert().Less(len(profile.Sample), len(allProfile.Sample))
					s.Assert().NotZero(len(profile.Sample))
				}
			}
		})
	}
}

func (s *testSuite) Test_BytesFetched_Populated() {
	// BytesFetched must always be populated in the response regardless of
	// whether diagnostics collection is enabled, and must be > 0 for any
	// query that actually reads block data.
	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Diagnostics)
	s.Require().NotNil(resp.Diagnostics.ExecutionNode)
	s.Require().NotNil(resp.Diagnostics.ExecutionNode.Stats)
	s.Assert().Greater(resp.Diagnostics.ExecutionNode.Stats.BytesFetched, uint64(0))
}

func (s *testSuite) Test_BytesFetched_ConsistentAcrossInvocations() {
	// Two independent Invoke calls with identical inputs must return a similar
	// BytesFetched value: the counter is scoped to a single invocation, so
	// neither retries from a higher layer nor shared bucket state can grossly
	// inflate it (which would roughly double the value).
	//
	// Exact byte equality is not achievable: the async parquet reader
	// (ReadModeAsync, used for blocks > 1 MB) spawns goroutines that issue
	// prefetch/read-ahead GetRange calls whose count and size are
	// timing-dependent. Two otherwise-identical invocations therefore read a
	// slightly different total number of physical bytes. A 10% relative
	// tolerance is ample to detect gross double-counting while tolerating
	// normal prefetch variance (~1-2% in practice).
	//
	// Use a fixed end time and clone the plan for each call: BlockReader.Invoke
	// mutates QueryPlan.Root.Blocks[i].Datasets in place (filterNotOwnedDatasets),
	// so sharing a plan across calls would cause different datasets to be processed
	// on the second invocation.
	endTime := time.Now().UnixMilli()
	invoke := func() uint64 {
		resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
			EndTime:       endTime,
			LabelSelector: "{}",
			QueryPlan:     s.plan.CloneVT(),
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TREE,
				Tree:      &queryv1.TreeQuery{MaxNodes: 16},
			}},
			Tenant: s.tenant,
		})
		s.Require().NoError(err)
		return resp.Diagnostics.ExecutionNode.Stats.BytesFetched
	}
	first := invoke()
	second := invoke()
	s.Assert().Greater(first, uint64(0))
	// Two identical queries must fetch a similar number of bytes (within 10%).
	s.Assert().InEpsilon(float64(first), float64(second), 0.10)
}

func (s *testSuite) Test_SpanAndTraceSelector_Combined_Errors() {
	// No public RPC sets both; both being set is an internal-plan bug.
	span := []string{fixtureMatchingSpanID}
	trace := []string{"0123456789abcdef0123456789abcdef"}

	for _, tt := range []struct {
		name  string
		query *queryv1.Query
	}{
		{"tree", &queryv1.Query{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16, SpanSelector: span, TraceIdSelector: trace},
		}},
		{"pprof", &queryv1.Query{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{SpanSelector: span, TraceIdSelector: trace},
		}},
	} {
		s.Run(tt.name, func() {
			_, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
				StartTime:     startTime.UnixMilli(),
				EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
				LabelSelector: "{}",
				QueryPlan:     s.plan,
				Query:         []*queryv1.Query{tt.query},
				Tenant:        s.tenant,
			})
			s.Require().Error(err)
			s.Require().Contains(err.Error(), "span_selector and trace_id_selector cannot be combined")
		})
	}
}

func (s *testSuite) Test_SpanSelector() {
	// Capture baselines used to verify that an empty/nil selector returns
	// the full (unfiltered) result.
	baselineTree, err := os.ReadFile("testdata/fixtures/tree_16.txt")
	s.Require().NoError(err)

	allTreeResp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	allTree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](allTreeResp.Reports[0].Tree.Tree)
	s.Require().NoError(err)

	allPprofResp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	var allProfile profilev1.Profile
	s.Require().NoError(pprof.Unmarshal(allPprofResp.Reports[0].Pprof.Pprof, &allProfile))

	tests := []struct {
		queryType    queryv1.QueryType
		name         string
		spanSelector []string
		wantErr      error
		want         string
	}{
		// Tree tests
		{queryv1.QueryType_QUERY_TREE, "tree/invalid span ID returns error", []string{"tooshort"}, errors.New(`invalid span id length: "tooshort"`), ""},
		{queryv1.QueryType_QUERY_TREE, "tree/empty selector returns baseline", []string{}, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_TREE, "tree/nil selector returns baseline", nil, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_TREE, "tree/non-matching span returns empty", []string{fixtureNonMatchingSpanID}, nil, spanSelectorWantEmpty},
		{queryv1.QueryType_QUERY_TREE, "tree/matching span filters result", []string{fixtureMatchingSpanID}, nil, spanSelectorWantFiltered},

		// Pprof tests
		{queryv1.QueryType_QUERY_PPROF, "pprof/invalid span ID returns error", []string{"tooshort"}, errors.New(`invalid span id length: "tooshort"`), ""},
		{queryv1.QueryType_QUERY_PPROF, "pprof/empty selector returns baseline", []string{}, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_PPROF, "pprof/nil selector returns baseline", nil, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_PPROF, "pprof/non-matching span returns empty", []string{fixtureNonMatchingSpanID}, nil, spanSelectorWantEmpty},
		{queryv1.QueryType_QUERY_PPROF, "pprof/matching span filters result", []string{fixtureMatchingSpanID}, nil, spanSelectorWantFiltered},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var (
				query                    *queryv1.Query
				reqStartTime, reqEndTime int64
			)

			if tt.queryType == queryv1.QueryType_QUERY_TREE {
				reqEndTime = time.Now().UnixMilli()
				query = &queryv1.Query{
					QueryType: queryv1.QueryType_QUERY_TREE,
					Tree: &queryv1.TreeQuery{
						MaxNodes:     16,
						SpanSelector: tt.spanSelector,
					},
				}
			} else {
				reqStartTime = startTime.UnixMilli()
				reqEndTime = startTime.Add(5 * time.Minute).UnixMilli()
				query = &queryv1.Query{
					QueryType: queryv1.QueryType_QUERY_PPROF,
					Pprof: &queryv1.PprofQuery{
						SpanSelector: tt.spanSelector,
					},
				}
			}

			resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
				StartTime:     reqStartTime,
				EndTime:       reqEndTime,
				LabelSelector: "{}",
				QueryPlan:     s.plan,
				Query:         []*queryv1.Query{query},
				Tenant:        s.tenant,
			})

			if tt.wantErr != nil {
				s.Require().Error(err)
				s.Require().EqualError(err, tt.wantErr.Error())
				s.Require().Nil(resp)
				return
			}

			s.Require().NoError(err)
			s.Require().NotNil(resp)
			s.Require().Len(resp.Reports, 1)

			if tt.queryType == queryv1.QueryType_QUERY_TREE {
				tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
				s.Require().NoError(err)

				switch tt.want {
				case spanSelectorWantBaseline:
					s.Assert().Equal(string(baselineTree), tree.String())
				case spanSelectorWantEmpty:
					s.Assert().Zero(tree.Total())
				case spanSelectorWantFiltered:
					s.Assert().NotZero(tree.Total())
					s.Assert().Less(tree.Total(), allTree.Total())
				default:
					s.Require().Fail("unknown span selector expectation", tt.want)
				}
			} else {
				var profile profilev1.Profile
				s.Require().NoError(pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &profile))

				switch tt.want {
				case spanSelectorWantBaseline:
					s.Assert().Equal(len(allProfile.Sample), len(profile.Sample))
				case spanSelectorWantEmpty:
					s.Assert().Zero(len(profile.Sample))
				case spanSelectorWantFiltered:
					s.Assert().NotZero(len(profile.Sample))
					s.Assert().Less(len(profile.Sample), len(allProfile.Sample))
				default:
					s.Require().Fail("unknown span selector expectation", tt.want)
				}
			}
		})
	}
}

func (s *testSuite) getProfileIDFromExemplars(t *testing.T) string {
	t.Helper()

	resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         30.0,
				ExemplarType: typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
			},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	// Find first exemplar with a profile ID
	for _, serie := range resp.Reports[0].TimeSeries.TimeSeries {
		for _, point := range serie.Points {
			if len(point.Exemplars) > 0 && point.Exemplars[0].ProfileId != "" {
				return point.Exemplars[0].ProfileId
			}
		}
	}
	s.Require().FailNow("no profile ID found in exemplars")
	return ""
}

const (
	fixtureMatchingTraceID    = "00000000000000000000000000000001"
	fixtureNonMatchingTraceID = "ffffffffffffffffffffffffffffffff"
)

func (s *testSuite) Test_TraceSelector() {
	baselineTree, err := os.ReadFile("testdata/fixtures/tree_16.txt")
	s.Require().NoError(err)

	allTreeResp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	allTree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](allTreeResp.Reports[0].Tree.Tree)
	s.Require().NoError(err)

	allPprofResp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       startTime.Add(5 * time.Minute).UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     &queryv1.PprofQuery{},
		}},
		Tenant: s.tenant,
	})
	s.Require().NoError(err)
	var allProfile profilev1.Profile
	s.Require().NoError(pprof.Unmarshal(allPprofResp.Reports[0].Pprof.Pprof, &allProfile))

	tests := []struct {
		queryType     queryv1.QueryType
		name          string
		traceSelector []string
		wantErr       error
		want          string
	}{
		// Tree tests
		{queryv1.QueryType_QUERY_TREE, "tree/invalid trace ID returns error", []string{"tooshort"}, errors.New(`invalid trace id length: "tooshort"`), ""},
		{queryv1.QueryType_QUERY_TREE, "tree/empty selector returns baseline", []string{}, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_TREE, "tree/nil selector returns baseline", nil, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_TREE, "tree/non-matching trace returns empty", []string{fixtureNonMatchingTraceID}, nil, spanSelectorWantEmpty},
		{queryv1.QueryType_QUERY_TREE, "tree/matching trace filters result", []string{fixtureMatchingTraceID}, nil, spanSelectorWantFiltered},

		// Pprof tests
		{queryv1.QueryType_QUERY_PPROF, "pprof/invalid trace ID returns error", []string{"tooshort"}, errors.New(`invalid trace id length: "tooshort"`), ""},
		{queryv1.QueryType_QUERY_PPROF, "pprof/empty selector returns baseline", []string{}, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_PPROF, "pprof/nil selector returns baseline", nil, nil, spanSelectorWantBaseline},
		{queryv1.QueryType_QUERY_PPROF, "pprof/non-matching trace returns empty", []string{fixtureNonMatchingTraceID}, nil, spanSelectorWantEmpty},
		{queryv1.QueryType_QUERY_PPROF, "pprof/matching trace filters result", []string{fixtureMatchingTraceID}, nil, spanSelectorWantFiltered},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var (
				query                    *queryv1.Query
				reqStartTime, reqEndTime int64
			)

			if tt.queryType == queryv1.QueryType_QUERY_TREE {
				reqEndTime = time.Now().UnixMilli()
				query = &queryv1.Query{
					QueryType: queryv1.QueryType_QUERY_TREE,
					Tree: &queryv1.TreeQuery{
						MaxNodes:        16,
						TraceIdSelector: tt.traceSelector,
					},
				}
			} else {
				reqStartTime = startTime.UnixMilli()
				reqEndTime = startTime.Add(5 * time.Minute).UnixMilli()
				query = &queryv1.Query{
					QueryType: queryv1.QueryType_QUERY_PPROF,
					Pprof: &queryv1.PprofQuery{
						TraceIdSelector: tt.traceSelector,
					},
				}
			}

			resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
				StartTime:     reqStartTime,
				EndTime:       reqEndTime,
				LabelSelector: "{}",
				QueryPlan:     s.plan,
				Query:         []*queryv1.Query{query},
				Tenant:        s.tenant,
			})

			if tt.wantErr != nil {
				s.Require().Error(err)
				s.Require().EqualError(err, tt.wantErr.Error())
				s.Require().Nil(resp)
				return
			}

			s.Require().NoError(err)
			s.Require().NotNil(resp)
			s.Require().Len(resp.Reports, 1)

			if tt.queryType == queryv1.QueryType_QUERY_TREE {
				tree, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Reports[0].Tree.Tree)
				s.Require().NoError(err)

				switch tt.want {
				case spanSelectorWantBaseline:
					s.Assert().Equal(string(baselineTree), tree.String())
				case spanSelectorWantEmpty:
					s.Assert().Zero(tree.Total())
				case spanSelectorWantFiltered:
					s.Assert().NotZero(tree.Total())
					s.Assert().Less(tree.Total(), allTree.Total())
				default:
					s.Require().Fail("unknown trace selector expectation", tt.want)
				}
			} else {
				var profile profilev1.Profile
				s.Require().NoError(pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &profile))

				switch tt.want {
				case spanSelectorWantBaseline:
					s.Assert().Equal(len(allProfile.Sample), len(profile.Sample))
				case spanSelectorWantEmpty:
					s.Assert().Zero(len(profile.Sample))
				case spanSelectorWantFiltered:
					s.Assert().NotZero(len(profile.Sample))
					s.Assert().Less(len(profile.Sample), len(allProfile.Sample))
				default:
					s.Require().Fail("unknown trace selector expectation", tt.want)
				}
			}
		})
	}
}
