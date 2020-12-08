package segment_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSegment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Segment Suite")
}
