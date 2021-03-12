package tree_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestTree(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tree Suite")
}
