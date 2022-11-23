package target_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTarget(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Target Suite")
}
