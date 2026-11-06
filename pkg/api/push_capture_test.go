package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	distributormodel "github.com/grafana/pyroscope/v2/pkg/distributor/model"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

type capturePolicyFunc func(string) profiledump.Policy

func (f capturePolicyFunc) ProfileDump(id string) profiledump.Policy { return f(id) }

type pushFunc func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)

func (f pushFunc) Push(ctx context.Context, r *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	return f(ctx, r)
}
func (f pushFunc) PushBatch(context.Context, *distributormodel.PushRequest) error {
	return errors.New("unexpected PushBatch")
}

type captureFixture struct {
	recorder *profiledump.Recorder
	registry *prometheus.Registry
	bucket   *objstore.InMemBucket
}

func capturePolicy(t *testing.T, selector string) profiledump.Policy {
	t.Helper()
	now := time.Now()
	probability := 1.0
	deadline := now.Add(time.Hour)
	p, err := (&profiledump.TenantConfig{ActiveUntil: &deadline, Selector: &selector, Probability: &probability}).Compile(now)
	require.NoError(t, err)
	require.True(t, p.ActiveAt(now))
	return p
}

type captureDependencies struct {
	Policies profiledump.PolicyProvider
	Upload   profiledump.UploadFunc
}

func newCaptureFixture(t *testing.T, policy profiledump.Policy, configure func(*profiledump.RecorderConfig, *captureDependencies)) *captureFixture {
	t.Helper()
	f := &captureFixture{registry: prometheus.NewRegistry(), bucket: objstore.NewInMemBucket()}
	cfg := profiledump.DefaultRecorderConfig()
	deps := captureDependencies{
		Policies: capturePolicyFunc(func(id string) profiledump.Policy {
			if id == "test" {
				return policy
			}
			return profiledump.Policy{}
		}),
		Upload: func(ctx context.Context, _, key string, body io.Reader) error {
			return f.bucket.Upload(ctx, key, body)
		},
	}
	if configure != nil {
		configure(&cfg, &deps)
	}
	var err error
	f.recorder, err = profiledump.NewRecorder(cfg, deps.Policies, deps.Upload, "test-distributor", log.NewNopLogger(), f.registry)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), f.recorder))
	t.Cleanup(func() { f.stop(t) })
	return f
}
func (f *captureFixture) stop(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, services.StopAndAwaitTerminated(ctx, f.recorder))
}
func (f *captureFixture) awaitUploads(t *testing.T) {
	t.Helper()
	want := f.counter(t, "pyroscope_profile_dump_captures_total", "enqueued")
	require.Eventually(t, func() bool {
		return f.counter(t, "pyroscope_profile_dump_uploads_total", "") == want
	}, 5*time.Second, time.Millisecond)
}
func (f *captureFixture) counter(t *testing.T, name, result string) float64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	var sum float64
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, m := range family.Metric {
			if result == "" {
				sum += m.GetCounter().GetValue()
				continue
			}
			for _, label := range m.Label {
				if label.GetName() == "result" && label.GetValue() == result {
					sum += m.GetCounter().GetValue()
				}
			}
		}
	}
	return sum
}
func sampleRequest() *connect.Request[pushv1.PushRequest] {
	r := connect.NewRequest(&pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
		Labels:  []*typesv1.LabelPair{{Name: "z", Value: "last"}, {Name: "service_name", Value: "checkout"}},
		Samples: []*pushv1.RawSample{{ID: "original-id", RawProfile: []byte("malformed pprof")}},
	}}})
	r.Header().Set("X-Private-Test", "not capture metadata")
	return r
}
func captureClient(t *testing.T, next pushv1connect.PusherServiceHandler, recorder *profiledump.Recorder, bodyLimitMB float64) pushv1connect.PusherServiceClient {
	t.Helper()
	a, router, _ := newTestAPIWithMode(t, AdminServerDisabled)
	a.cfg.GrpcAuthMiddleware = connect.WithInterceptors(tenant.NewAuthInterceptor(true))
	limits := validation.MockOverrides(func(defaults *validation.Limits, _ map[string]*validation.Limits) {
		defaults.IngestionBodyLimitMB = bodyLimitMB
	})
	a.registerPusher(next, limits, recorder)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	opts := append(connectapi.DefaultClientOptions(), connect.WithInterceptors(tenant.NewAuthInterceptor(true)))
	return pushv1connect.NewPusherServiceClient(srv.Client(), srv.URL, opts...)
}

func TestCapturePusherOwnershipAndResponse(t *testing.T) {
	f := newCaptureFixture(t, capturePolicy(t, "{}"), nil)
	req := sampleRequest()
	req.Msg.Series = append(req.Msg.Series, &pushv1.RawProfileSeries{Samples: []*pushv1.RawSample{{RawProfile: []byte("second")}}})
	expected := connect.NewResponse(&pushv1.PushResponse{})
	expectedErr := connect.NewError(connect.CodeInvalidArgument, errors.New("downstream failure"))
	calls := 0
	next := pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		assert.Same(t, req, got)
		assert.Len(t, got.Msg.Series, 2)
		for _, series := range got.Msg.Series {
			clear(series.Samples[0].RawProfile)
		}
		return expected, expectedErr
	})
	got, err := (&capturePusher{next: next, recorder: f.recorder}).Push(tenant.InjectTenantID(context.Background(), "test"), req)
	require.Same(t, expected, got)
	require.Same(t, expectedErr, err)
	require.Equal(t, 1, calls)
	f.awaitUploads(t)
	var payloads [][]byte
	for key, data := range f.bucket.Objects() {
		if strings.HasSuffix(key, ".pprof") {
			payloads = append(payloads, data)
		}
	}
	require.ElementsMatch(t, [][]byte{[]byte("malformed pprof"), []byte("second")}, payloads)
}

func TestCapturePusherTransport(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		large, panicDownstream bool
		nilRecorder            bool
		code                   connect.Code
		captures, calls        int
	}{
		{name: "success", captures: 1, calls: 1},
		{name: "nil recorder", nilRecorder: true, calls: 1},
		{name: "outer read limit", large: true, code: connect.CodeResourceExhausted},
		{name: "recovery", panicDownstream: true, code: connect.CodeInternal, captures: 1, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCaptureFixture(t, capturePolicy(t, "{}"), nil)
			var calls atomic.Int32
			next := pushFunc(func(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				calls.Add(1)
				id, err := tenant.ExtractTenantIDFromContext(ctx)
				assert.NoError(t, err)
				assert.Equal(t, "test", id)
				if tc.panicDownstream {
					panic("synthetic downstream panic")
				}
				resp := connect.NewResponse(&pushv1.PushResponse{})
				resp.Header().Set("X-Result", "preserved")
				resp.Trailer().Set("X-Trailer", "preserved")
				return resp, nil
			})
			recorder := f.recorder
			if tc.nilRecorder {
				recorder = nil
			}
			client := captureClient(t, next, recorder, 0.001)
			ctx := tenant.InjectTenantID(context.Background(), "test")
			req := sampleRequest()
			if tc.large {
				req.Msg.Series[0].Samples[0].RawProfile = bytes.Repeat([]byte("x"), 2048)
			}
			resp, err := client.Push(ctx, req)
			if tc.code == 0 {
				require.NoError(t, err)
				require.Equal(t, "preserved", resp.Header().Get("X-Result"))
				require.Equal(t, "preserved", resp.Trailer().Get("X-Trailer"))
			} else {
				require.Equal(t, tc.code, connect.CodeOf(err))
			}
			require.Equal(t, int32(tc.calls), calls.Load())
			f.awaitUploads(t)
			require.Len(t, f.bucket.Objects(), 2*tc.captures)
			require.Equal(t, float64(tc.captures), f.counter(t, "pyroscope_profile_dump_captures_total", ""))
		})
	}
}

func TestCapturePusherContinuesWhenCapacityIsFull(t *testing.T) {
	started := make(chan struct{})
	f := newCaptureFixture(t, capturePolicy(t, "{}"), func(c *profiledump.RecorderConfig, d *captureDependencies) {
		c.MaxCaptureBytes, c.MaxRetainedBytes = 1024, 1024
		d.Upload = func(ctx context.Context, _, _ string, _ io.Reader) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
	})
	expected := connect.NewResponse(&pushv1.PushResponse{})
	calls := 0
	wrapper := &capturePusher{recorder: f.recorder, next: pushFunc(func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		return expected, nil
	})}
	req := sampleRequest()
	req.Msg.Series[0].Samples[0].RawProfile = make([]byte, 512)
	ctx := tenant.InjectTenantID(context.Background(), "test")
	got, err := wrapper.Push(ctx, req)
	require.NoError(t, err)
	require.Same(t, expected, got)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upload not started")
	}
	// A second request must reach ingestion while the first upload holds capacity.
	got, err = wrapper.Push(ctx, req)
	require.NoError(t, err)
	require.Same(t, expected, got)
	require.Equal(t, 2, calls)
	require.Equal(t, 1., f.counter(t, "pyroscope_profile_dump_captures_total", "byte_budget"))
}

func TestCapturePusherMissingTenant(t *testing.T) {
	f := newCaptureFixture(t, capturePolicy(t, "{}"), nil)
	req := sampleRequest()
	calls := 0
	expectedErr := errors.New("downstream result")
	next := pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		assert.Same(t, req, got)
		return nil, expectedErr
	})
	_, err := (&capturePusher{next: next, recorder: f.recorder}).Push(context.Background(), req)
	require.Same(t, expectedErr, err)
	require.Equal(t, 1, calls)
	require.Zero(t, f.counter(t, "pyroscope_profile_dump_captures_total", ""))
}

func BenchmarkCapturePusherDisabled(b *testing.B) {
	for _, state := range []string{"nil", "disabled", "expired"} {
		b.Run(state, func(b *testing.B) {
			var recorder *profiledump.Recorder
			if state != "nil" {
				var policy profiledump.Policy
				var err error
				if state == "expired" {
					now := time.Now()
					deadline := now
					probability := 1.0
					policy, err = (&profiledump.TenantConfig{ActiveUntil: &deadline, Probability: &probability}).Compile(now)
					require.NoError(b, err)
				}
				recorder, err = profiledump.NewRecorder(profiledump.DefaultRecorderConfig(),
					capturePolicyFunc(func(string) profiledump.Policy { return policy }),
					func(context.Context, string, string, io.Reader) error { return nil },
					"benchmark", log.NewNopLogger(), prometheus.NewRegistry())
				require.NoError(b, err)
				require.NoError(b, services.StartAndAwaitRunning(context.Background(), recorder))
				defer func() { require.NoError(b, services.StopAndAwaitTerminated(context.Background(), recorder)) }()
			}
			response := connect.NewResponse(&pushv1.PushResponse{})
			next := pushFunc(func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				return response, nil
			})
			wrapper := &capturePusher{next: next, recorder: recorder}
			ctx := tenant.InjectTenantID(context.Background(), "test")
			req := sampleRequest()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = wrapper.Push(ctx, req)
			}
		})
	}
}

func TestCapturePusherTenantSelection(t *testing.T) {
	checkout := capturePolicy(t, `{service_name="checkout"}`)
	other := capturePolicy(t, `{service_name="other"}`)
	f := newCaptureFixture(t, checkout, func(_ *profiledump.RecorderConfig, d *captureDependencies) {
		d.Policies = capturePolicyFunc(func(id string) profiledump.Policy {
			if id == "test" {
				return checkout
			}
			return other
		})
	})
	calls := 0
	req := sampleRequest()
	wrapper := &capturePusher{recorder: f.recorder, next: pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	})}
	for _, id := range []string{"test", "other"} {
		for _, value := range []string{"checkout", "other"} {
			req.Msg.Series[0].Labels[1].Value = value
			_, err := wrapper.Push(tenant.InjectTenantID(context.Background(), id), req)
			require.NoError(t, err)
		}
	}
	require.Equal(t, 4, calls)
	f.awaitUploads(t)
	type selection struct{ tenant, service string }
	var selections []selection
	for key, data := range f.bucket.Objects() {
		if strings.HasSuffix(key, ".json") {
			var metadata profiledump.Metadata
			require.NoError(t, json.Unmarshal(data, &metadata))
			selections = append(selections, selection{metadata.TenantID, metadata.Labels[1].Value})
		}
	}
	require.ElementsMatch(t, []selection{{"test", "checkout"}, {"other", "other"}}, selections)
}
