package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/protobuf/proto"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	distributormodel "github.com/grafana/pyroscope/v2/pkg/distributor/model"
	legacy "github.com/grafana/pyroscope/v2/pkg/ingester/pyroscope"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

var captureNow = time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

type capturePolicyFunc func(string) profiledump.Policy

func (f capturePolicyFunc) ProfileDebugDump(id string) profiledump.Policy { return f(id) }

type pushFunc func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error)

func (f pushFunc) Push(ctx context.Context, r *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	return f(ctx, r)
}
func (f pushFunc) PushBatch(context.Context, *distributormodel.PushRequest) error {
	return errors.New("unexpected PushBatch")
}

type capturedObject struct {
	metadata profiledump.Metadata
	payload  []byte
	key      string
}
type captureFixture struct {
	recorder *profiledump.Recorder
	registry *prometheus.Registry
	mu       sync.Mutex
	objects  []capturedObject
}

func capturePolicy(t *testing.T, selector string, probability float64, expired bool) profiledump.Policy {
	t.Helper()
	deadline := captureNow.Add(time.Hour)
	if expired {
		deadline = captureNow
	}
	rate := 10.0
	p, err := (&profiledump.TenantConfig{ActiveUntil: &deadline, Selector: &selector, Probability: &probability, MaxCapturesPerSecond: &rate}).Compile(profiledump.DefaultConfig(), captureNow)
	require.NoError(t, err)
	return p
}
func newCaptureFixture(t *testing.T, policy profiledump.Policy, configure func(*profiledump.RecorderConfig, *profiledump.Dependencies)) *captureFixture {
	t.Helper()
	f := &captureFixture{registry: prometheus.NewRegistry()}
	cfg := profiledump.DefaultRecorderConfig()
	cfg.TenantBurst, cfg.ProcessBurst, cfg.QueueCapacity, cfg.Workers = 100, 100, 100, 1
	deps := profiledump.Dependencies{
		Policies: capturePolicyFunc(func(id string) profiledump.Policy {
			if id == "test" {
				return policy
			}
			return profiledump.Policy{}
		}),
		DistributorID: "test-distributor", Registerer: f.registry, Now: func() time.Time { return captureNow },
		Upload: func(_ context.Context, id, key string, body io.Reader) error {
			var payload bytes.Buffer
			metadata, err := profiledump.Decode(body, &payload, cfg.MaxObjectBytes)
			if err != nil {
				return err
			}
			assert.Equal(t, id, metadata.TenantID)
			f.mu.Lock()
			defer f.mu.Unlock()
			f.objects = append(f.objects, capturedObject{metadata, payload.Bytes(), key})
			return nil
		},
	}
	if configure != nil {
		configure(&cfg, &deps)
	}
	var err error
	f.recorder, err = profiledump.NewRecorder(cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { f.drain(t) })
	return f
}
func (f *captureFixture) drain(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.recorder.Shutdown(ctx))
}
func (f *captureFixture) captured(t *testing.T) []capturedObject {
	t.Helper()
	f.drain(t)
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedObject(nil), f.objects...)
}
func (f *captureFixture) counter(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	var sum float64
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, m := range family.Metric {
			matched := true
			for k, v := range labels {
				found := false
				for _, l := range m.Label {
					if l.GetName() == k && l.GetValue() == v {
						found = true
					}
				}
				matched = matched && found
			}
			if matched {
				sum += m.GetCounter().GetValue()
			}
		}
	}
	return sum
}
func sampleRequest() *connect.Request[pushv1.PushRequest] {
	r := connect.NewRequest(&pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
		Labels:  []*typesv1.LabelPair{{Name: "z", Value: "last"}, {Name: "service_name", Value: "checkout"}},
		Samples: []*pushv1.RawSample{{ID: "original-id", RawProfile: []byte("malformed native pprof")}},
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

func TestCapturePusherSelection(t *testing.T) {
	for _, tc := range []struct {
		name, selector    string
		probability       float64
		disabled, expired bool
		random            []float64
		want              int
		reason            string
	}{
		{name: "disabled", disabled: true, reason: "disabled"},
		{name: "expired", selector: "{}", probability: 1, expired: true, reason: "expired"},
		{name: "all samples", selector: "{}", probability: 1, want: 4},
		{name: "external selector", selector: `{service_name="checkout"}`, probability: 1, want: 2, reason: "selector"},
		{name: "selector miss", selector: `{service_name="missing"}`, probability: 1, reason: "selector"},
		{name: "independent sampling", selector: "{}", probability: 0.5, random: []float64{0.9, 0.1, 0.8, 0.2}, want: 2, reason: "sampled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var policy profiledump.Policy
			if !tc.disabled {
				policy = capturePolicy(t, tc.selector, tc.probability, tc.expired)
			}
			draws := 0
			f := newCaptureFixture(t, policy, func(_ *profiledump.RecorderConfig, d *profiledump.Dependencies) {
				d.Random = func() float64 { v := tc.random[draws]; draws++; return v }
			})
			req := sampleRequest()
			req.Msg.Series[0].Samples = append(req.Msg.Series[0].Samples, &pushv1.RawSample{ID: "second", RawProfile: []byte("two")})
			second := proto.Clone(req.Msg.Series[0]).(*pushv1.RawProfileSeries)
			second.Labels[1].Value = "different"
			second.Samples[0].ID, second.Samples[1].ID = "third", "fourth"
			req.Msg.Series = append(req.Msg.Series, second)
			before := proto.Clone(req.Msg)
			calls := 0
			expected := connect.NewResponse(&pushv1.PushResponse{})
			next := pushFunc(func(ctx context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				calls++
				assert.Same(t, req, got)
				assert.True(t, proto.Equal(before, got.Msg))
				assert.Equal(t, "not capture metadata", got.Header().Get("X-Private-Test"))
				return expected, nil
			})
			got, err := (&capturePusher{next: next, recorder: f.recorder}).Push(tenant.InjectTenantID(context.Background(), "test"), req)
			require.NoError(t, err)
			require.Same(t, expected, got)
			require.Equal(t, 1, calls)
			objects := f.captured(t)
			require.Len(t, objects, tc.want)
			for _, object := range objects {
				found := false
				for _, series := range req.Msg.Series {
					for _, sample := range series.Samples {
						if sample.ID == object.metadata.OriginalProfileID {
							found = true
							require.Equal(t, sample.RawProfile, object.payload)
							require.Equal(t, captureLabels(series.Labels), object.metadata.Labels)
						}
					}
				}
				require.True(t, found, "captured sample ID must belong to the original request")
			}
			require.Equal(t, len(tc.random), draws)
			if tc.reason != "" {
				wantDrops := float64(4 - tc.want)
				if tc.disabled || tc.expired {
					wantDrops = 0 // No candidates are constructed for inactive requests.
				}
				require.Equal(t, wantDrops, f.counter(t, "pyroscope_profile_dump_dropped_total", map[string]string{"reason": tc.reason}))
			}
			if tc.name == "independent sampling" {
				require.Equal(t, "second", objects[0].metadata.OriginalProfileID)
				require.Equal(t, "fourth", objects[1].metadata.OriginalProfileID)
			}
		})
	}
}

func TestCapturePusherOwnershipAndResponse(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			release := make(chan struct{})
			started := make(chan struct{})
			f := newCaptureFixture(t, capturePolicy(t, "{}", 1, false), func(_ *profiledump.RecorderConfig, d *profiledump.Dependencies) {
				upload := d.Upload
				d.Upload = func(ctx context.Context, id, key string, r io.Reader) error {
					close(started)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					return upload(ctx, id, key, r)
				}
			})
			defer close(release)
			req := sampleRequest()
			var compressed bytes.Buffer
			gz := gzip.NewWriter(&compressed)
			_, err := gz.Write(req.Msg.Series[0].Samples[0].RawProfile)
			require.NoError(t, err)
			require.NoError(t, gz.Close())
			req.Msg.Series[0].Samples[0].RawProfile = compressed.Bytes()
			original := bytes.Clone(compressed.Bytes())
			expected := connect.NewResponse(&pushv1.PushResponse{})
			expected.Header().Set("X-Result", "same")
			var expectedErr error
			if fail {
				expectedErr = connect.NewError(connect.CodeInvalidArgument, errors.New("downstream failure"))
			}
			ctx, cancel := context.WithCancel(tenant.InjectTenantID(context.Background(), "test"))
			defer cancel()
			calls := 0
			next := pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				calls++
				assert.Same(t, req, got)
				assert.Equal(t, 1.0, f.counter(t, "pyroscope_profile_dump_candidates_total", map[string]string{"result": "enqueued"}))
				clear(got.Msg.Series[0].Samples[0].RawProfile)
				got.Msg.Series[0].Samples[0].ID = "changed"
				got.Msg.Series[0].Labels[1].Value = "changed"
				cancel()
				return expected, expectedErr
			})
			got, err := (&capturePusher{next: next, recorder: f.recorder}).Push(ctx, req)
			require.Same(t, expected, got)
			require.Equal(t, expectedErr, err)
			require.Equal(t, 1, calls)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("upload not started")
			}
			// Unblock without closing twice; the deferred close permits cleanup on failure.
			release <- struct{}{}
			objects := f.captured(t)
			require.Len(t, objects, 1)
			require.Equal(t, original, objects[0].payload)
			require.Equal(t, "original-id", objects[0].metadata.OriginalProfileID)
			require.Equal(t, map[string]string{"z": "last", "service_name": "checkout"}, objects[0].metadata.Labels)
			require.Equal(t, "gzip", objects[0].metadata.PayloadEncoding)
			require.Equal(t, profiledump.SourceConnect, objects[0].metadata.SourceProtocol)
		})
	}
}

func TestCapturePusherTransport(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		auth, large, panicDownstream bool
		code                         connect.Code
		captures, calls              int
	}{
		{name: "success", auth: true, captures: 1, calls: 1},
		{name: "authentication", code: connect.CodeUnauthenticated},
		{name: "outer read limit", auth: true, large: true, code: connect.CodeResourceExhausted},
		{name: "recovery", auth: true, panicDownstream: true, code: connect.CodeInternal, captures: 1, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCaptureFixture(t, capturePolicy(t, "{}", 1, false), nil)
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
			client := captureClient(t, next, f.recorder, 0.001)
			ctx := context.Background()
			if tc.auth {
				ctx = tenant.InjectTenantID(ctx, "test")
			}
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
			require.Len(t, f.captured(t), tc.captures)
			require.Equal(t, float64(tc.captures), f.counter(t, "pyroscope_profile_dump_candidates_total", nil))
		})
	}
}

func TestCapturePusherLegacyBypass(t *testing.T) {
	f := newCaptureFixture(t, capturePolicy(t, "{}", 1, false), nil)
	calls := 0
	next := pushFunc(func(_ context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		for _, s := range req.Msg.Series {
			for _, sample := range s.Samples {
				_, err := pprof.RawFromBytes(sample.RawProfile)
				require.NoError(t, err)
			}
		}
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	})
	// Register the external wrapper around the same downstream service used by the
	// real legacy adapter; its line-format conversion calls Push, not PushBatch.
	_ = captureClient(t, next, f.recorder, 1)
	handler := legacy.NewPyroscopeIngestHandler(next, validation.MockOverrides(func(*validation.Limits, map[string]*validation.Limits) {}), log.NewNopLogger())
	req := httptest.NewRequest(http.MethodPost, "/ingest?name=checkout&format=lines", strings.NewReader("main;work 1\n"))
	req = req.WithContext(tenant.InjectTenantID(req.Context(), "test"))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Equal(t, 1, calls)
	require.Empty(t, f.captured(t))
	require.Zero(t, f.counter(t, "pyroscope_profile_dump_candidates_total", nil))
}

func TestCapturePusherQueueAndUploadFailure(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var uploads atomic.Int32
	f := newCaptureFixture(t, capturePolicy(t, "{}", 1, false), func(c *profiledump.RecorderConfig, d *profiledump.Dependencies) {
		c.QueueCapacity = 1
		d.Upload = func(ctx context.Context, _, _ string, _ io.Reader) error {
			if uploads.Add(1) == 1 {
				close(started)
			}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return errors.New("synthetic store failure")
		}
	})
	defer close(release)
	calls := 0
	next := pushFunc(func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	})
	wrapper := &capturePusher{next: next, recorder: f.recorder}
	ctx := tenant.InjectTenantID(context.Background(), "test")
	_, err := wrapper.Push(ctx, sampleRequest())
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("upload not started")
	}
	for range 2 {
		_, err = wrapper.Push(ctx, sampleRequest())
		require.NoError(t, err)
	}
	require.Equal(t, 3, calls)
	require.Equal(t, 1.0, f.counter(t, "pyroscope_profile_dump_dropped_total", map[string]string{"reason": "queue_full"}))
	release <- struct{}{}
	release <- struct{}{}
	f.drain(t)
	require.Equal(t, 2.0, f.counter(t, "pyroscope_profile_dump_upload_errors_total", nil))
}

func TestCapturePusherSpans(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	for _, tc := range []struct {
		name, selector string
		probability    float64
		burst          int
		objectBytes    int64
		want           int
		reason         string
	}{
		{name: "selector miss", selector: `{service_name="other"}`, probability: 1},
		{name: "sampling miss", selector: "{}", probability: 0.5},
		{name: "enqueued", selector: "{}", probability: 1, want: 2},
		{name: "rate", selector: "{}", probability: 1, burst: 1, want: 1},
		{name: "size drop", selector: "{}", probability: 1, objectBytes: 100, want: 2, reason: "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(spans.Ended())
			f := newCaptureFixture(t, capturePolicy(t, tc.selector, tc.probability, false), func(c *profiledump.RecorderConfig, d *profiledump.Dependencies) {
				d.Random = func() float64 { return 0.9 }
				if tc.burst != 0 {
					c.TenantBurst = tc.burst
				}
				if tc.objectBytes != 0 {
					c.MaxObjectBytes = tc.objectBytes
				}
			})
			ctx, parent := provider.Tracer("test").Start(tenant.InjectTenantID(context.Background(), "test"), "parent")
			defer parent.End()
			next := pushFunc(func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				assert.Len(t, spans.Ended(), before+tc.want)
				return connect.NewResponse(&pushv1.PushResponse{}), nil
			})
			req := sampleRequest()
			req.Msg.Series[0].Samples = append(req.Msg.Series[0].Samples, req.Msg.Series[0].Samples[0])
			_, err := (&capturePusher{next: next, recorder: f.recorder}).Push(ctx, req)
			require.NoError(t, err)
			objects := f.captured(t)
			ended := spans.Ended()[before:]
			require.Len(t, ended, tc.want)
			for _, span := range ended {
				require.Equal(t, "Profile.Capture", span.Name())
				require.Equal(t, parent.SpanContext().SpanID(), span.Parent().SpanID())
				attrs := map[attribute.Key]attribute.Value{}
				for _, a := range span.Attributes() {
					attrs[a.Key] = a.Value
				}
				require.NotEmpty(t, attrs["capture.id"].AsString())
				require.NotEmpty(t, attrs["capture.object_key"].AsString())
				require.NotEmpty(t, attrs["capture.policy_fingerprint"].AsString())
				require.Equal(t, "pprof", attrs["capture.format"].AsString())
				require.Equal(t, "connect", attrs["capture.source"].AsString())
				require.NotEmpty(t, attrs["capture.distributor_id"].AsString())
				require.Positive(t, attrs["capture.payload_size"].AsInt64())
				require.Equal(t, tc.reason, attrs["capture.drop_reason"].AsString())
				result := "enqueued"
				if tc.reason != "" {
					result = "dropped"
				}
				require.Equal(t, result, attrs["capture.result"].AsString())
				if result == "enqueued" {
					found := false
					for _, object := range objects {
						if object.key == attrs["capture.object_key"].AsString() {
							found = true
							require.Equal(t, object.metadata.CaptureID, attrs["capture.id"].AsString())
						}
					}
					require.True(t, found, "trace must identify the uploaded object")
				}
			}
		})
	}
}

func TestCapturePusherMetadataBounds(t *testing.T) {
	f := newCaptureFixture(t, capturePolicy(t, `{selector_only="hit"}`, 1, false), nil)
	req := sampleRequest()
	for i := 0; i < profiledump.MaxLabels+1; i++ {
		req.Msg.Series[0].Labels = append(req.Msg.Series[0].Labels, &typesv1.LabelPair{Name: fmt.Sprintf("label_%02d", i), Value: strings.Repeat("v", 300)})
	}
	req.Msg.Series[0].Labels = append(req.Msg.Series[0].Labels, &typesv1.LabelPair{Name: "selector_only", Value: "hit"})
	before := proto.Clone(req.Msg)
	next := pushFunc(func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	})
	_, err := (&capturePusher{next: next, recorder: f.recorder}).Push(tenant.InjectTenantID(context.Background(), "test"), req)
	require.NoError(t, err)
	require.True(t, proto.Equal(before, req.Msg))
	objects := f.captured(t)
	require.Len(t, objects, 1)
	require.NoError(t, objects[0].metadata.Validate())
	require.LessOrEqual(t, len(objects[0].metadata.Labels), profiledump.MaxLabels)
}

func TestCapturePusherSelectsOversizedLabelValue(t *testing.T) {
	f := newCaptureFixture(t, capturePolicy(t, `{oversized=~"x+",service_name="checkout"}`, 1, false), nil)
	req := sampleRequest()
	req.Msg.Series[0].Labels = append(req.Msg.Series[0].Labels, &typesv1.LabelPair{Name: "oversized", Value: strings.Repeat("x", 4<<20)})
	want := bytes.Clone(req.Msg.Series[0].Samples[0].RawProfile)
	nextErr := connect.NewError(connect.CodeInvalidArgument, errors.New("malformed pprof"))
	wrapper := &capturePusher{recorder: f.recorder, next: pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		require.Same(t, req, got)
		// Input reuse after capture must not change the owned upload.
		got.Msg.Series[0].Labels[1].Value = "reused"
		clear(got.Msg.Series[0].Samples[0].RawProfile)
		return nil, nextErr
	})}
	_, err := wrapper.Push(tenant.InjectTenantID(context.Background(), "test"), req)
	require.Same(t, nextErr, err)
	objects := f.captured(t)
	require.Len(t, objects, 1)
	require.Equal(t, want, objects[0].payload)
	require.Equal(t, "checkout", objects[0].metadata.Labels["service_name"])
	require.NotContains(t, objects[0].metadata.Labels, "oversized")
}

func TestCapturePusherNilRecorderAndMissingTenant(t *testing.T) {
	for _, missingTenant := range []bool{false, true} {
		t.Run(fmt.Sprint(missingTenant), func(t *testing.T) {
			var recorder *profiledump.Recorder
			if missingTenant {
				recorder = newCaptureFixture(t, capturePolicy(t, "{}", 1, false), nil).recorder
			}
			req := sampleRequest()
			calls := 0
			expectedErr := errors.New("downstream result")
			next := pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				calls++
				assert.Same(t, req, got)
				return nil, expectedErr
			})
			ctx := context.Background()
			if !missingTenant {
				ctx = tenant.InjectTenantID(ctx, "test")
			}
			_, err := (&capturePusher{next: next, recorder: recorder}).Push(ctx, req)
			require.Same(t, expectedErr, err)
			require.Equal(t, 1, calls)
		})
	}
}

func TestCapturePusherOptionalProfileID(t *testing.T) {
	for _, tc := range []struct{ name, id, want string }{
		{"empty", "", ""},
		{"ordinary", "original-id", "original-id"},
		{"maximum", strings.Repeat("x", profiledump.MaxTextBytes), strings.Repeat("x", profiledump.MaxTextBytes)},
		{"oversized", strings.Repeat("x", profiledump.MaxTextBytes+1), ""},
		{"control character", "bad\nID", ""},
		{"invalid UTF-8", "\xff", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCaptureFixture(t, capturePolicy(t, "{}", 1, false), nil)
			req := sampleRequest()
			req.Msg.Series[0].Samples[0].ID = tc.id
			before := proto.Clone(req.Msg)
			nextErr := connect.NewError(connect.CodeInvalidArgument, errors.New("downstream failure"))
			next := pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				require.True(t, proto.Equal(before, got.Msg))
				return nil, nextErr
			})
			_, err := (&capturePusher{next: next, recorder: f.recorder}).Push(tenant.InjectTenantID(context.Background(), "test"), req)
			require.Same(t, nextErr, err)
			objects := f.captured(t)
			require.Len(t, objects, 1)
			require.Equal(t, tc.want, objects[0].metadata.OriginalProfileID)
			require.Equal(t, req.Msg.Series[0].Samples[0].RawProfile, objects[0].payload)
		})
	}
}

func BenchmarkCapturePusherDisabled(b *testing.B) {
	for _, state := range []string{"nil", "disabled", "expired"} {
		b.Run(state, func(b *testing.B) {
			var recorder *profiledump.Recorder
			if state != "nil" {
				var policy profiledump.Policy
				if state == "expired" {
					deadline := captureNow
					probability := 1.0
					var err error
					policy, err = (&profiledump.TenantConfig{ActiveUntil: &deadline, Probability: &probability}).Compile(profiledump.DefaultConfig(), captureNow)
					require.NoError(b, err)
				}
				var err error
				recorder, err = profiledump.NewRecorder(profiledump.DefaultRecorderConfig(), profiledump.Dependencies{
					Policies:      capturePolicyFunc(func(string) profiledump.Policy { return policy }),
					DistributorID: "benchmark", Registerer: prometheus.NewRegistry(),
					Upload: func(context.Context, string, string, io.Reader) error { return nil },
				})
				require.NoError(b, err)
				defer func() { require.NoError(b, recorder.Shutdown(context.Background())) }()
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

func TestCapturePusherUnrepresentableMetadataLabels(t *testing.T) {
	for _, value := range []string{"line1\nline2", "tab\tvalue", "null\x00value", "unicode\u0085control"} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			// The selector must still see the exact value that metadata cannot encode.
			f := newCaptureFixture(t, capturePolicy(t, fmt.Sprintf("{description=%q}", value), 1, false), nil)
			req := sampleRequest()
			req.Msg.Series[0].Labels = append(req.Msg.Series[0].Labels,
				&typesv1.LabelPair{Name: "description", Value: value},
				&typesv1.LabelPair{Name: "bad\nname", Value: "omitted"},
				&typesv1.LabelPair{Name: "later", Value: "preserved"},
			)
			before := proto.Clone(req.Msg)
			var calls atomic.Int32
			next := pushFunc(func(_ context.Context, got *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
				calls.Add(1)
				assert.True(t, proto.Equal(before, got.Msg))
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("synthetic downstream parsing failure"))
			})
			client := captureClient(t, next, f.recorder, 1)
			_, err := client.Push(tenant.InjectTenantID(context.Background(), "test"), req)
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			require.Equal(t, int32(1), calls.Load())
			objects := f.captured(t)
			require.Len(t, objects, 1)
			require.Equal(t, req.Msg.Series[0].Samples[0].RawProfile, objects[0].payload)
			require.Equal(t, "original-id", objects[0].metadata.OriginalProfileID)
			require.Equal(t, map[string]string{"z": "last", "service_name": "checkout", "later": "preserved"}, objects[0].metadata.Labels)
			require.NoError(t, objects[0].metadata.Validate())
			require.Zero(t, f.counter(t, "pyroscope_profile_dump_dropped_total", nil))
		})
	}
}

func TestCapturePusherPolicyReload(t *testing.T) {
	active := capturePolicy(t, "{}", 1, false)
	var current atomic.Pointer[profiledump.Policy]
	disabled := profiledump.Policy{}
	current.Store(&disabled)
	f := newCaptureFixture(t, active, func(_ *profiledump.RecorderConfig, d *profiledump.Dependencies) {
		d.Policies = capturePolicyFunc(func(string) profiledump.Policy { return *current.Load() })
	})
	calls := 0
	next := pushFunc(func(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
		calls++
		return connect.NewResponse(&pushv1.PushResponse{}), nil
	})
	wrapper := &capturePusher{next: next, recorder: f.recorder}
	ctx := tenant.InjectTenantID(context.Background(), "test")
	for _, policy := range []*profiledump.Policy{&disabled, &active, &disabled} {
		current.Store(policy)
		_, err := wrapper.Push(ctx, sampleRequest())
		require.NoError(t, err)
	}
	require.Equal(t, 3, calls)
	require.Len(t, f.captured(t), 1)
}
