package names_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNames(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Names Suite")
}
