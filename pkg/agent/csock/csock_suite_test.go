package csock_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCsock(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Csock Suite")
}
