package pprof_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPprof(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pprof Suite")
}
