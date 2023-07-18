package flamebearer_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	testing2 "github.com/grafana/pyroscope/pkg/og/testing"
)

func TestFlamebearer(t *testing.T) {
	testing2.SetupLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Flambearer Suite")
}
