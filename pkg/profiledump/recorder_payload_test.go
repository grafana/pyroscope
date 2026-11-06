package profiledump

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestRecorderPayloads(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"empty", []byte{}}, {"malformed", []byte{0, 0xff, 1}}, {"malformed gzip", []byte{0x1f, 0x8b, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			var bodies [][]byte
			r, err := NewRecorder(DefaultRecorderConfig(), &policySet{}, func(_ context.Context, _, key string, body io.Reader) error {
				data, err := io.ReadAll(body)
				keys = append(keys, key)
				bodies = append(bodies, data)
				return err
			}, "test", log.NewNopLogger(), nil)
			require.NoError(t, err)
			item, reason := r.prepareCapture("a", captureMetadata(), tc.payload, recorderNow)
			require.Empty(t, reason)
			defer r.release(item)
			r.upload(context.Background(), item)
			require.Equal(t, []string{item.key, item.metadataKey}, keys)
			require.Equal(t, tc.payload, bodies[0])
			var metadata Metadata
			require.NoError(t, json.Unmarshal(bodies[1], &metadata))
			require.Equal(t, int64(len(tc.payload)), metadata.PayloadSize)
		})
	}
}

func TestRecorderUploadTimeoutStartsAfterQueueWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, proceed := make(chan struct{}), make(chan struct{})
		unblock := sync.OnceFunc(func() { close(proceed) })
		var payloads int
		r, _, _ := recorderFixture(t, recorderTestConfig(), func(ctx context.Context, _, key string, _ io.Reader) error {
			if strings.HasSuffix(key, ".pprof") {
				payloads++
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.Equal(t, 10*time.Second, time.Until(deadline))
				if payloads == 1 {
					close(started)
					<-proceed
				}
			}
			return nil
		})
		t.Cleanup(unblock)
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), nil, r.now()))
		await(t, started)
		require.Empty(t, r.capture(context.Background(), "a", captureMetadata(), nil, r.now()))
		time.Sleep(3 * time.Second)
		require.Len(t, r.queue, 1)
		unblock()
		synctest.Wait()
		require.Equal(t, 2, payloads)
		require.Equal(t, 2., testutil.ToFloat64(r.metrics.uploads.WithLabelValues("success")))
	})
}

func TestRecorderPairUploadResults(t *testing.T) {
	for _, tc := range []struct {
		name, result    string
		stage           int
		failure         error
		timeout, cancel bool
		calls           int
	}{
		{name: "complete", result: "success", calls: 2},
		{name: "payload failure", result: "error", stage: 1, failure: errors.New("payload failed"), calls: 1},
		{name: "sidecar failure", result: "error", stage: 2, failure: errors.New("sidecar failed"), calls: 2},
		{name: "unexpected cancellation", result: "error", stage: 1, failure: context.Canceled, calls: 1},
		{name: "deadline error", result: "timeout", stage: 1, failure: context.DeadlineExceeded, calls: 1},
		{name: "sidecar timeout after acceptance", result: "timeout", stage: 2, timeout: true, calls: 2},
		{name: "sidecar success after cancellation", stage: 2, cancel: true, calls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var logs bytes.Buffer
				var keys []string
				deadline := time.Now().Add(recorderUploadTimeout)
				r, err := NewRecorder(DefaultRecorderConfig(), &policySet{}, func(ctx context.Context, _, key string, _ io.Reader) error {
					keys = append(keys, key)
					got, ok := ctx.Deadline()
					require.True(t, ok)
					require.Equal(t, deadline, got, "both objects share the upload deadline")
					if len(keys) == tc.stage {
						if tc.timeout {
							<-ctx.Done()
						}
						if tc.cancel {
							cancel()
						}
						return tc.failure
					}
					time.Sleep(time.Second)
					return nil
				}, "test", log.NewLogfmtLogger(&logs), nil)
				require.NoError(t, err)
				metadata := captureMetadata()
				metadata.Labels = []*typesv1.LabelPair{{Name: "private-label", Value: "private-value"}}
				item, reason := r.prepareCapture("a", metadata, []byte("private-payload"), recorderNow)
				require.Empty(t, reason)
				defer r.release(item)
				r.upload(ctx, item)
				require.Equal(t, []string{item.key, item.metadataKey}[:tc.calls], keys)
				if tc.result == "" {
					require.Zero(t, testutil.CollectAndCount(r.metrics.uploads))
					require.Empty(t, logs.String())
				} else {
					require.Equal(t, 1., testutil.ToFloat64(r.metrics.uploads.WithLabelValues(tc.result)))
					require.Equal(t, 1, testutil.CollectAndCount(r.metrics.uploads))
				}
				if tc.name == "sidecar failure" {
					for _, field := range []string{"level=error", "operation=sidecar", "tenant=a", "key=" + item.metadataKey, "err="} {
						require.Contains(t, logs.String(), field)
					}
					for _, private := range []string{"private-payload", "private-label", "private-value"} {
						require.NotContains(t, logs.String(), private)
					}
				}
			})
		})
	}
}

func TestRecorderRetainedBudgetBoundary(t *testing.T) {
	metadata, payload := captureMetadata(), make([]byte, 512)
	probe, err := NewRecorder(DefaultRecorderConfig(), &policySet{}, discardUpload, "test", log.NewNopLogger(), nil)
	require.NoError(t, err)
	prepared, reason := probe.prepareCapture("a", metadata, payload, recorderNow)
	require.Empty(t, reason)
	probe.release(prepared)
	size := int64(len(payload) + len(prepared.metadataJSON))
	for _, tc := range []struct {
		name              string
		capture, retained int64
		want              dropReason
	}{
		{"one byte short", size, 2*size - 1, dropByteBudget},
		{"exact fit", size, 2 * size, ""},
		{"maximum budget", math.MaxInt64, math.MaxInt64, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewRecorder(RecorderConfig{MaxCaptureBytes: tc.capture, MaxRetainedBytes: tc.retained}, &policySet{}, discardUpload, "test", log.NewNopLogger(), nil)
			require.NoError(t, err)
			first, reason := r.prepareCapture("a", metadata, payload, recorderNow)
			require.Empty(t, reason)
			defer r.release(first)
			second, reason := r.prepareCapture("a", metadata, payload, recorderNow)
			if reason == "" {
				defer r.release(second)
			}
			require.Equal(t, tc.want, reason)
		})
	}
}
