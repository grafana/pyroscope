package integration

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/pprof/testhelper"
)

func TestV2HistoricalBackfill(t *testing.T) {
	p := new(PyroscopeTest).Configure(t, true)
	p.start(t)
	t.Cleanup(p.stop)

	profileTime := time.Now().Add(-8 * 7 * 24 * time.Hour).Truncate(time.Second)
	profile := testhelper.NewProfileBuilder(profileTime.UnixNano()).
		CPUProfile().
		ForStacktraceString("historical", "work").
		AddSamples(239)
	rawProfile, err := profile.MarshalVT()
	require.NoError(t, err)

	rb := p.NewRequestBuilder(t)
	rb.Push(rb.PushPPROFRequestFromBytes(rawProfile, "process_cpu"), 200, "")

	req := connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Start:         profileTime.Add(-time.Minute).UnixMilli(),
		End:           profileTime.Add(time.Minute).UnixMilli(),
		LabelSelector: `{service_name="` + rb.AppName + `"}`,
	})
	require.Eventually(t, func() bool {
		resp, err := rb.QueryClient().SelectMergeProfile(context.Background(), req)
		return err == nil && len(resp.Msg.Sample) > 0
	}, 10*time.Second, 100*time.Millisecond)
}
