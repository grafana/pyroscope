package cappedarr_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCappedarr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cappedarr Suite")
}
