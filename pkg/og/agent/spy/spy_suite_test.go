package spy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSpy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spy Suite")
}
