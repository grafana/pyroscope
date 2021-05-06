package debug_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDebug(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Debug Suite")
}
