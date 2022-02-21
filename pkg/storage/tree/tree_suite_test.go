package tree_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	testing2 "github.com/pyroscope-io/pyroscope/pkg/testing"
)

func TestTree(t *testing.T) {
	testing2.SetupLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tree Suite")
}
