package profiledump

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

func TestRecorderSeriesSelection(t *testing.T) {
	for _, tc := range []struct {
		name, selector string
		probability    float64
		expired        bool
		result         string
		count          float64
		draws          int
	}{
		{name: "disabled"},
		{name: "expired", selector: "{}", probability: 1, expired: true},
		{name: "selector miss", selector: `{service_name="other"}`, probability: 1, result: string(dropSelector), count: 2},
		{name: "sampled out", selector: "{}", probability: .5, result: string(dropSampled), count: 2, draws: 1},
		{name: "selected", selector: `{service_name="checkout"}`, probability: .75, result: resultEnqueued, count: 2, draws: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draws := 0
			cfg := recorderTestConfig()
			cfg.ProcessBurst = 2
			r, policies, clock := recorderFixture(t, cfg, discardUpload)
			r.random = func() float64 {
				draws++
				// A reload during sampling must not change the rest of this series.
				policies.set("a", Policy{})
				return .5
			}
			policy := Policy{}
			if tc.probability != 0 {
				policy = recorderPolicy(t, tc.selector, tc.probability)
			}
			policies.set("a", policy)
			if tc.expired {
				clock.advance(time.Hour)
			}
			series := &pushv1.RawProfileSeries{
				Labels:  []*typesv1.LabelPair{{Name: "service_name", Value: "checkout"}},
				Samples: []*pushv1.RawSample{nil, {RawProfile: []byte("one")}, {RawProfile: []byte("two")}},
			}
			r.CaptureSeries(context.Background(), "a", series)
			if tc.result == "" {
				require.Zero(t, testutil.CollectAndCount(r.metrics.captures))
			} else {
				require.Equal(t, tc.count, testutil.ToFloat64(r.metrics.captures.WithLabelValues(tc.result)))
			}
			require.Equal(t, tc.draws, draws)
			if tc.result != resultEnqueued && !tc.expired {
				// Rejected selection must leave the process allowance for later work.
				policies.set("a", recorderPolicy(t, "{}", 1))
				r.CaptureSeries(context.Background(), "a", series)
				require.Equal(t, 2., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
			}
		})
	}
}

func TestRecorderSeriesSampling(t *testing.T) {
	r, policies, _ := recorderFixture(t, recorderTestConfig(), discardUpload)
	draws := 0
	r.random = func() float64 {
		draws++
		if draws%2 == 1 {
			return .9
		}
		return .1
	}
	policies.set("a", recorderPolicy(t, "{}", .5))
	series := &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{}, {}}}
	for range 4 {
		r.CaptureSeries(context.Background(), "a", series)
	}
	require.Equal(t, 4, draws)
	require.Equal(t, 4., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
	require.Equal(t, 4., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(dropSampled))))
}

func TestRecorderSeriesProfileLimits(t *testing.T) {
	for _, reason := range []dropReason{dropProcessRate, dropTooLarge} {
		t.Run(string(reason), func(t *testing.T) {
			cfg := recorderTestConfig()
			if reason == dropProcessRate {
				cfg.ProcessBurst = 2
			}
			cfg.MaxCaptureBytes = 1024
			r, _, _ := recorderFixture(t, cfg, discardUpload)
			series := &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{
				{RawProfile: []byte("one")}, {RawProfile: []byte("two")}, {RawProfile: []byte("three")},
			}}
			if reason == dropTooLarge {
				series.Labels = []*typesv1.LabelPair{{Name: "label", Value: strings.Repeat("x", 200)}}
				series.Samples[1].RawProfile = make([]byte, 900)
			}
			r.CaptureSeries(context.Background(), "a", series)
			require.Equal(t, 2., testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(reason))))
		})
	}
}

func TestRecorderCaptureSizeAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, id, distributorID string
		payload                 []byte
		labels                  []*typesv1.LabelPair
	}{
		{name: "payload", payload: make([]byte, 2048)},
		{name: "labels", labels: []*typesv1.LabelPair{{Name: "large", Value: strings.Repeat("x", 2048)}}},
		{name: "profile ID", id: strings.Repeat("p", 2048)},
		{name: "distributor ID", distributorID: strings.Repeat("d", 2048)},
		// The input fits, but JSON escaping pushes the serialized capture over the limit.
		{name: "serialized metadata", labels: []*typesv1.LabelPair{{Name: "escaped", Value: strings.Repeat("<", 200)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.MaxCaptureBytes = 1024
			r, _, _ := recorderFixture(t, cfg, discardUpload)
			r.distributorID = tc.distributorID
			r.CaptureSeries(context.Background(), "a", &pushv1.RawProfileSeries{
				Labels: tc.labels, Samples: []*pushv1.RawSample{{RawProfile: tc.payload, ID: tc.id}},
			})
			require.Equal(t, 1., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(dropTooLarge))))
			require.Zero(t, testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
			require.Zero(t, testutil.CollectAndCount(r.metrics.uploads))
		})
	}
}

func TestRecorderInvalidAdmission(t *testing.T) {
	r, policies, _ := recorderFixture(t, recorderTestConfig(), discardUpload)
	policies.set("", recorderPolicy(t, "{}", 1))
	r.CaptureSeries(context.Background(), "", &pushv1.RawProfileSeries{
		Samples: []*pushv1.RawSample{{}},
	})
	require.Equal(t, 1., testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(dropInvalid))))
	require.Equal(t, 1, testutil.CollectAndCount(r.metrics.captures))
	require.Zero(t, testutil.CollectAndCount(r.metrics.uploads))
}

func TestRecorderRejectionSkipsMetadata(t *testing.T) {
	for _, reason := range []dropReason{dropTooLarge, dropProcessRate} {
		t.Run(string(reason), func(t *testing.T) {
			cfg := recorderTestConfig()
			cfg.MaxCaptureBytes = 1024
			r, _, _ := recorderFixture(t, cfg, discardUpload)
			payload := []byte("profile")
			if reason == dropTooLarge {
				payload = make([]byte, 2048)
			} else {
				require.True(t, r.process.AllowN(r.now(), cfg.ProcessBurst))
			}
			series := &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{RawProfile: payload}}}
			for i := range 32 {
				series.Labels = append(series.Labels, &typesv1.LabelPair{Name: fmt.Sprintf("label_%d", i), Value: "value"})
			}
			policy := r.policies.ProfileDump("a")
			selection := testing.AllocsPerRun(100, func() { r.selectSeries(policy, model.Labels(series.Labels)) })
			allocations := testing.AllocsPerRun(100, func() { r.CaptureSeries(context.Background(), "a", series) })
			require.Equal(t, selection, allocations, "rejection must avoid key and metadata allocation")
			require.Positive(t, testutil.ToFloat64(r.metrics.captures.WithLabelValues(string(reason))))
			require.Zero(t, testutil.ToFloat64(r.metrics.captures.WithLabelValues(resultEnqueued)))
		})
	}
}

func TestRecorderEmptySeries(t *testing.T) {
	r, _, _ := recorderFixture(t, recorderTestConfig(), discardUpload)
	var disabled *Recorder
	for _, series := range []*pushv1.RawProfileSeries{nil, {}, {Samples: []*pushv1.RawSample{nil}}} {
		r.CaptureSeries(context.Background(), "a", series)
		disabled.CaptureSeries(context.Background(), "a", series)
	}
	require.Zero(t, testutil.CollectAndCount(r.metrics.captures))
}
