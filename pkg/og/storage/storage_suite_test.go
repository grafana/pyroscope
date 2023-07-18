package storage_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	testutils "github.com/grafana/pyroscope/pkg/og/testing"
)

func TestStorage(t *testing.T) {
	testutils.SetupLogging()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage Suite", types.ReporterConfig{
		SlowSpecThreshold: 60 * time.Second,
	})
}
