package upstream_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUpstream(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upstream Suite")
}
