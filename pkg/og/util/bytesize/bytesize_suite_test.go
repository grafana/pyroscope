package bytesize_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBytesize(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bytesize Suite")
}
