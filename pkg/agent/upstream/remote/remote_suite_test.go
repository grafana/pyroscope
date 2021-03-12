package remote_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRemote(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Remote Suite")
}
