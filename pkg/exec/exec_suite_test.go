package exec_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExec(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exec Suite")
}
