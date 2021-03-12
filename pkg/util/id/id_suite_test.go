package id_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestId(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Id Suite")
}
