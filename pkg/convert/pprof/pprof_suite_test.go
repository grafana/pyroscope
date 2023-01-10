package pprof_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestConvert(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pprof Suite")
}
