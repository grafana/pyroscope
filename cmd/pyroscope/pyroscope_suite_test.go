package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPyroscope(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pyroscope Suite")
}
