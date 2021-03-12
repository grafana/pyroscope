package convert_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestConvert(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Convert Suite")
}
