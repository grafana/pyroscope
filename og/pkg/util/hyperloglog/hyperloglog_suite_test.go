package hyperloglog_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHyperloglog(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hyperloglog Suite")
}
