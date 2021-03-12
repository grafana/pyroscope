package varint_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVarint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Varint Suite")
}
