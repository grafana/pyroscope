package tree_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"

	testing2 "github.com/petethepig/pyroscope/pkg/testing"
)

func TestTree(t *testing.T) {
	testing2.SetupLogging()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tree Suite")
}
