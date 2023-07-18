package dimension_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDimension(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dimension Suite")
}
