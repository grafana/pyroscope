package transporttrie_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTransporttrie(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Transporttrie Suite")
}
