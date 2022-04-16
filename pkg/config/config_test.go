package config_test

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

var _ = Describe("config", func() {
	It("Should have equal or omitted mapstructure and yaml tags", func() {
		Expect("a").To(Equal("a"))
		s := config.Server{}
		IterateStructTags(s, func(st reflect.StructTag) {
			ms := st.Get("mapstructure")
			y := st.Get("yaml")
			Expect(ms).To(BeEqualOrOmitted(y))
		})
	})
})

func IterateStructTags(p interface{}, callback func(reflect.StructTag)) {
	s := reflect.ValueOf(p)
	stype := reflect.TypeOf(p)
	if s.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < stype.NumField(); i++ {
		f := s.Field(i)
		ftype := stype.Field(i)

		if f.Kind() == reflect.Struct {
			IterateStructTags(f.Interface(), callback)
		} else {
			callback(ftype.Tag)
		}
	}

}

type StructTagValue struct {
	Value string
}

func BeEqualOrOmitted(a string) types.GomegaMatcher {
	return &StructTagValue{Value: a}
}

func (p *StructTagValue) Match(actual interface{}) (bool, error) {
	switch actual := actual.(type) {
	case string:
		return actual == p.Value || actual == "-" || p.Value == "-", nil
	}
	return false, nil
}

func (p *StructTagValue) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected tag to be %v but received %v", actual, p.Value)
}

func (p *StructTagValue) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected tag to be %v but received %v", actual, p.Value)
}
