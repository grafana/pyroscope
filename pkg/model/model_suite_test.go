package model_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Suite")
}

func expectErrOrNil(actual, expected error) {
	assertion := Expect(actual)
	if expected == nil {
		assertion.To(BeNil())
		return
	}
	assertion.To(MatchError(expected))
}
