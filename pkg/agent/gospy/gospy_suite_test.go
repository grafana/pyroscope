package gospy_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGoSpy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GoSpy Suite")
}
