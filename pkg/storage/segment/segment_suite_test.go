package segment_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSegment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Segment Suite")
}
