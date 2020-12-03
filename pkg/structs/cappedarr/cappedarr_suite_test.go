package cappedarr_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestCappedarr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cappedarr Suite")
}
