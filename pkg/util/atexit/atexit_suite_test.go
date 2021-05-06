package atexit_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAtexit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Atexit Suite")
}
