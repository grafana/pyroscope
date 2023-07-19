package tree_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	testing2 "github.com/grafana/pyroscope/pkg/og/testing"
)

func TestTree(t *testing.T) {
	testing2.SetupLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tree Suite")
}
