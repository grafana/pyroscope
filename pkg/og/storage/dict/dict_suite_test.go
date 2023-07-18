package dict_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	testing2 "github.com/grafana/pyroscope/pkg/og/testing"
)

func TestTdict(t *testing.T) {
	testing2.SetupLogging()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Tdict Suite")
}
