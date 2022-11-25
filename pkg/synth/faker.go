package synth

import (
	"fmt"
)

type functionNameGenerator struct {
	mapping map[string]string
}

func newFunctionNameGenerator() *functionNameGenerator {
	return &functionNameGenerator{
		mapping: make(map[string]string),
	}
}

type fakerStruct struct {
	Word string `faker:"word,unique"`
}

func (f *functionNameGenerator) functionName(name string) string {
	if _, ok := f.mapping[name]; !ok {
		f.mapping[name] = fmt.Sprintf("fn_%d", len(f.mapping))
	}
	return f.mapping[name]
}
