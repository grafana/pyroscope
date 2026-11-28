package querybackend

import (
	"time"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// Expected totals were independently verified from the checked-in blocks' full rows.
// Both CPU types are included so nanoseconds and sample counts must stay separate.
func (s *testSuite) Test_QuerySpanPresence() {
	end := time.Now().UnixMilli()
	for _, tc := range []struct {
		name          string
		spans, traces []string
		labels        string
		start, end    int64
		want          []*querierv1.SpanPresence
	}{
		{
			name:   "span",
			spans:  []string{"0000000000000001"},
			labels: `{__profile_type__=~"process_cpu:.*"}`, end: end,
			want: []*querierv1.SpanPresence{
				{TraceId: "00000000000000000000000000000001", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 130590000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 12840},
				}},
				{TraceId: "00000000000000000000000000000002", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 113520000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 10999},
				}},
				{TraceId: "00000000000000000000000000000003", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 131330000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 12796},
				}},
				{TraceId: "00000000000000000000000000000004", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 116110000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11027},
				}},
				{TraceId: "00000000000000000000000000000005", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 134210000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 12835},
				}},
				{TraceId: "00000000000000000000000000000006", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 112470000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11109},
				}},
				{TraceId: "00000000000000000000000000000007", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 130660000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 12904},
				}},
				{TraceId: "00000000000000000000000000000008", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 109990000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11364},
				}},
				{TraceId: "00000000000000000000000000000009", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 128010000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 13144},
				}},
				{TraceId: "0000000000000000000000000000000a", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 110140000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11604},
				}},
				{TraceId: "0000000000000000000000000000000b", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 128240000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 13410},
				}},
				{TraceId: "0000000000000000000000000000000c", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 111150000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11237},
				}},
				{TraceId: "0000000000000000000000000000000d", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 131690000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 13057},
				}},
				{TraceId: "0000000000000000000000000000000e", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 110320000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11236},
				}},
				{TraceId: "0000000000000000000000000000000f", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 127840000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 13097},
				}},
				{TraceId: "00000000000000000000000000000010", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 112900000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 11325},
				}},
			},
		},
		{
			name:   "trace",
			traces: []string{"00000000000000000000000000000001"},
			labels: `{__profile_type__=~"process_cpu:.*"}`, end: end,
			want: []*querierv1.SpanPresence{
				{TraceId: "00000000000000000000000000000001", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 130590000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 12840},
				}},
				{TraceId: "00000000000000000000000000000001", SpanId: "0000000000000002", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 112950000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 10998},
				}},
			},
		},
		{
			name:  "combined",
			spans: []string{"0000000000000001"}, traces: []string{"00000000000000000000000000000001"},
			labels: `{__profile_type__=~"process_cpu:.*"}`, end: end,
			want: []*querierv1.SpanPresence{
				{TraceId: "00000000000000000000000000000001", SpanId: "0000000000000001", Totals: []*querierv1.SpanProfileTotal{
					{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", Total: 130590000000},
					{ProfileTypeID: "process_cpu:samples:count:cpu:nanoseconds", Total: 12840},
				}},
			},
		},
		{
			name:  "non-matching trace",
			spans: []string{"0000000000000001"}, traces: []string{"ffffffffffffffffffffffffffffffff"},
			labels: `{__profile_type__=~"process_cpu:.*"}`, end: end,
		},
		{
			name: "non-matching span", spans: []string{"ffffffffffffffff"},
			labels: `{__profile_type__=~"process_cpu:.*"}`, end: end,
		},
		{
			name: "non-matching labels", spans: []string{"0000000000000001"},
			labels: `{service_name="nonexistent-span-presence-service"}`, end: end,
		},
		{
			name: "non-matching time range", spans: []string{"0000000000000001"},
			labels: `{__profile_type__=~"process_cpu:.*"}`, start: 1, end: 2,
		},
	} {
		s.Run(tc.name, func() {
			resp, err := s.reader.Invoke(s.ctx, &queryv1.InvokeRequest{
				StartTime: tc.start, EndTime: tc.end, LabelSelector: tc.labels,
				QueryPlan: s.plan, Tenant: s.tenant,
				Query: []*queryv1.Query{{
					QueryType:    queryv1.QueryType_QUERY_SPAN_PRESENCE,
					SpanPresence: &queryv1.SpanPresenceQuery{SpanSelector: tc.spans, TraceIdSelector: tc.traces},
				}},
			})
			s.Require().NoError(err)
			var got []*querierv1.SpanPresence
			if len(resp.Reports) > 0 {
				s.Require().Len(resp.Reports, 1)
				got = resp.Reports[0].SpanPresence.Spans
			}
			s.ElementsMatch(tc.want, got)
		})
	}
}
