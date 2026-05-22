package querybackend

import (
	"fmt"
	"sync"
	"time"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// Test_LabelValues_NoMatcher_BufferReuse verifies that label values returned
// by the no-matcher fast path remain valid after the dataset's TSDB buffer has
// been recycled to bufferpool.
//
// Reader.LabelValues (empty matchers) produces strings via
// yoloString(d.UvarintBytes()), which aliases the pooled TSDB buffer.
// When the dataset is closed the buffer is returned to bufferpool.Put.
// A concurrent request can then receive the same buffer from
// bufferpool.GetBuffer and overwrite it while the first request's strings are
// still in use (e.g. being inserted into the LabelMerger map or sorted during
// aggregation), resulting in a data race.
//
// Run with -race to exercise the race detector.  Without the strings.Clone fix
// in queryLabelValues the detector catches the concurrent read (aggregator) vs
// write (bufferpool reuse) on the same backing array.  With the fix each value
// is a heap copy independent of the buffer lifetime.
func (s *testSuite) Test_LabelValues_NoMatcher_BufferReuse() {
	endTime := time.Now().UnixMilli()

	// baseReq returns a fresh InvokeRequest with its own cloned QueryPlan.
	// BlockReader.Invoke mutates QueryPlan.Root.Blocks[i].Datasets in place
	// (filterNotOwnedDatasets), so concurrent callers must not share a plan.
	baseReq := func() *queryv1.InvokeRequest {
		return &queryv1.InvokeRequest{
			EndTime:       endTime,
			LabelSelector: "{}",
			QueryPlan:     s.plan.CloneVT(),
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_LABEL_VALUES,
				LabelValues: &queryv1.LabelValuesQuery{
					LabelName: "service_name",
				},
			}},
			Tenant: s.tenant,
		}
	}

	// Sequential reference call to establish the expected label values.
	refResp, err := s.reader.Invoke(s.ctx, baseReq())
	s.Require().NoError(err)
	s.Require().Len(refResp.Reports, 1)
	want := refResp.Reports[0].LabelValues.LabelValues
	s.Require().NotEmpty(want, "test requires at least one service_name label value in the fixture data")

	const (
		workers    = 20
		iterations = 20
	)

	type callResult struct {
		got []string
		err error
	}
	// Buffered so goroutines never block on send.
	results := make(chan callResult, workers*iterations)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for range iterations {
				resp, err := s.reader.Invoke(s.ctx, baseReq())
				if err != nil {
					results <- callResult{err: err}
					return
				}
				if len(resp.Reports) == 0 {
					results <- callResult{err: fmt.Errorf("no reports returned")}
					return
				}
				// Reading the strings here races with concurrent buffer
				// recycling if yoloString aliases were not cloned.
				results <- callResult{got: resp.Reports[0].LabelValues.LabelValues}
			}
		}()
	}
	wg.Wait()
	close(results)

	// Check all results in the test goroutine (s.Require/Assert are not
	// safe to call from non-test goroutines).
	for r := range results {
		s.Require().NoError(r.err)
		s.Assert().Equal(want, r.got)
	}
}
