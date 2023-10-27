package ebpfspy

import (
	"testing"

	"github.com/efficientgo/e2e"
	"github.com/stretchr/testify/require"
)

func TestPythonVersions(t *testing.T) {
	e, err := e2e.New()
	require.NoError(t, err)
	t.Cleanup(e.Close)

	j := e.Runnable("tracing").
		WithPorts(
			map[string]int{
				"http.front":    16686,
				"jaeger.thrift": 14268,
			}).
		Init(e2e.StartOptions{Image: "jaegertracing/all-in-one:1.25"})
}
