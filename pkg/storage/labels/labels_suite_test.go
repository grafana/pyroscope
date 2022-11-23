package labels_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLabels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Labels Suite")
}
