package pyroscope

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof/bench"
)

type MockPushService struct {
}

func (m *MockPushService) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	//fmt.Printf("pushing %d profiles\n", len(req.Msg.Series))
	res := &connect.Response[pushv1.PushResponse]{
		Msg: &pushv1.PushResponse{},
	}
	return res, nil
}

func BenchmarkIngestJFR(b *testing.B) {
	testdataDir := "../../../pkg/og/convert/jfr/testdata"
	jfrs := []string{
		//"cortex-dev-01__kafka-0__cpu__0.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu__1.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu__2.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu__3.jfr.gz",
		"cortex-dev-01__kafka-0__cpu_lock0_alloc0__0.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu_lock_alloc__0.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu_lock_alloc__1.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu_lock_alloc__2.jfr.gz",
		//"cortex-dev-01__kafka-0__cpu_lock_alloc__3.jfr.gz",
	}
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	h := NewPyroscopeIngestHandler(&MockPushService{}, l)

	for _, jfr := range jfrs {
		b.Run(jfr, func(b *testing.B) {
			jfr, err := bench.ReadGzipFile(testdataDir + "/" + jfr)
			require.NoError(b, err)
			for i := 0; i < b.N; i++ {
				res := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/ingest?name=javaapp&format=jfr", bytes.NewReader(jfr))
				req.Header.Set("Content-Type", "application/octet-stream")
				h.ServeHTTP(res, req)
			}
		})
	}

}
