package direct_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDirect(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Direct Suite")
}
