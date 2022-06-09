package distributor

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/pprof"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/grafana/fire/pkg/gen/ingester/v1/ingestv1connect"
	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
	"github.com/grafana/fire/pkg/gen/push/v1/pushv1connect"
)

type fakeIngester struct {
	t testing.TB
}

func (i *fakeIngester) Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	res := connect.NewResponse(&pushv1.PushResponse{})
	return res, nil
}

func newFakeIngester(t testing.TB) ingestv1connect.IngesterClient {
	return &fakeIngester{t: t}
}

func Test_ConnectPush(t *testing.T) {
	mux := http.NewServeMux()
	d, err := New(Config{}, nil, log.NewLogfmtLogger(os.Stdout))
	require.NoError(t, err)
	d.client = newFakeIngester(t)
	mux.Handle(pushv1connect.NewPusherHandler(d))
	s := httptest.NewServer(mux)
	defer s.Close()

	client := pushv1connect.NewPusherClient(http.DefaultClient, s.URL)

	resp, err := client.Push(context.Background(), connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: []*pushv1.LabelPair{
					{Name: "cluster", Value: "us-central1"},
				},
				Samples: []*pushv1.RawSample{
					{
						RawProfile: testProfile(t),
					},
				},
			},
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func testProfile(t *testing.T) []byte {
	t.Helper()

	buf := bytes.NewBuffer(nil)
	require.NoError(t, pprof.WriteHeapProfile(buf))
	return buf.Bytes()
}
