package synth

import (
	"fmt"
	"regexp"
	"strings"
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

func simpleName(s string) string {
	s = strings.ToLower(s)
	r := regexp.MustCompile("[^a-z0-9_]")
	s = r.ReplaceAllString(s, "")
	return s
}

func (f *functionNameGenerator) functionName(name string) string {
	if _, ok := f.mapping[name]; !ok {
		arr := strings.Split(name, " - ")
		newName := simpleName(arr[len(arr)-1])
		if len(newName) == 0 {
			newName = "empty"
		}
		// newName += "_" + fmt.Sprintf("fn_%d", len(f.mapping))
		// f.mapping[name] = newName
		f.mapping[name] = fmt.Sprintf("fn_%d_%s", len(f.mapping), newName)
	}
	return f.mapping[name]
}
