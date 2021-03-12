package caps_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCaps(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Caps Suite")
}
