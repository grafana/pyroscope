package profiler_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestProfiler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Profiler Suite")
}
