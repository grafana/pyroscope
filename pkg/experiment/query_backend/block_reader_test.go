package query_backend

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	"github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/test"
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
}

func (s *testSuite) SetupSuite() {
	s.bucket = memory.NewInMemBucket()
	s.loadFromDir(s.dir)
}

func (s *testSuite) SetupTest() {
	s.ctx = context.Background()
	s.logger = test.NewTestingLogger(s.T())
	s.reader = NewBlockReader(s.logger, &objstore.ReaderAtBucket{Bucket: s.bucket})
	s.meta = make([]*metastorev1.BlockMeta, len(s.blocks))
	for i, b := range s.blocks {
		s.meta[i] = b.CloneVT()
	}
	s.sanitizeMetadata()
	s.plan = query_plan.Build(s.meta, 10, 10)
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
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	tree, err := phlaremodel.UnmarshalTree(resp.Reports[0].Tree.Tree)
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
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	tree, err := phlaremodel.UnmarshalTree(resp.Reports[0].Tree.Tree)
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
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)

	expected, err := os.ReadFile("testdata/fixtures/series_labels.json")
	s.Require().NoError(err)
	actual, _ := json.Marshal(resp.Reports[0].SeriesLabels)
	s.Assert().JSONEq(string(expected), string(actual))
}

func (s *testSuite) Test_QueryTimeSeries() {
	query := &queryv1.Query{
		QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
		TimeSeries: &queryv1.TimeSeriesQuery{
			GroupBy: []string{"service_name"},
			Step:    1.0, // 1 second step
		},
	}

	req := &queryv1.InvokeRequest{
		StartTime:     time.Now().Add(-1 * time.Hour).UnixMilli(),
		EndTime:       time.Now().UnixMilli(),
		Query:         []*queryv1.Query{query},
		QueryPlan:     s.plan,
		LabelSelector: "{}",
	}

	resp, err := s.reader.Invoke(s.ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Reports, 1)
	s.Require().NotNil(resp.Reports[0].TimeSeries)
}
