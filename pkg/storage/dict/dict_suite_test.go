package dict_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	testing2 "github.com/pyroscope-io/pyroscope/pkg/testing"
)

func TestTdict(t *testing.T) {
	testing2.SetupLogging()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Tdict Suite")
}
