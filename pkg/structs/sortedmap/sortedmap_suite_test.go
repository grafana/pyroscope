package sortedmap_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSortedmap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sortedmap Suite")
}
