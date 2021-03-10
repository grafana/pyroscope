package hyperloglog_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestHyperloglog(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hyperloglog Suite")
}
