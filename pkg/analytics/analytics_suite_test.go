package analytics_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAnalytics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Analytics Suite")
}
