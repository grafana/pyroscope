package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

func TestExporterClientMetrics(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "okay")
	}))
	defer svr.Close()

	// replace default metrics
	reg := prometheus.NewRegistry()
	oldReg := prometheus.DefaultRegisterer
	defer func() {
		prometheus.DefaultRegisterer = oldReg
	}()

	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		log.Fatal(err)
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	c, err := newClient(svr.URL, "user", "password", newMetrics(reg))
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, c.Store(ctx, []byte("xyz"), 1234))

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader("")))

}
