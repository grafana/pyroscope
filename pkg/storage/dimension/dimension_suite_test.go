package dimension_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDimension(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dimension Suite")
}
