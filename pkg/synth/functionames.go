package synth

import (
	"fmt"
	"regexp"
	"strings"
)

var reserved = []string{
	"alias",
	"and",
	"begin",
	"break",
	"case",
	"class",
	"def",
	"to_s",
	"defined",
	"do",
	"else",
	"elsif",
	"end",
	"true",
	"ensure",
	"false",
	"for",
	"if",
	"in",
	"module",
	"next",
	"nil",
	"not",
	"or",
	"redo",
	"require",
	"require_relative",
	"initialize",
	"rescue",
	"retry",
	"return",
	"self",
	"super",
	"then",
	"true",
	"undef",
	"unless",
	"until",
	"when",
	"while",
	"yield",
	"method_missing",
}

type functionNameSanitizer struct {
	mapping map[string]string
}

func newFunctionNameSanitizer() *functionNameSanitizer {
	return &functionNameSanitizer{
		mapping: make(map[string]string),
	}
}

func simpleName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	r := regexp.MustCompile("[^a-z0-9_]")
	s = r.ReplaceAllString(s, "")
	return s
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}

func (f *functionNameSanitizer) sanitize(name string) string {
	if _, ok := f.mapping[name]; !ok {
		newName := simpleName(name)
		if len(newName) == 0 {
			newName = "empty"
		}
		if contains(reserved, newName) {
			newName += "_"
		}
		f.mapping[name] = fmt.Sprintf("%s", newName)
	}
	return f.mapping[name]
}
