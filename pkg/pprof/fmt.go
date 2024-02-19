package pprof

import (
	"regexp"
	"strings"
)

var goStructTypeParameterRegex = regexp.MustCompile(`\[go\.shape\..*\]`)

func DropGoTypeParameters(input string) string {
	matchesIndices := goStructTypeParameterRegex.FindAllStringIndex(input, -1)
	if len(matchesIndices) == 0 {
		return input
	}
	var modified strings.Builder
	prevEnd := 0
	for _, indices := range matchesIndices {
		start, end := indices[0], indices[1]
		modified.WriteString(input[prevEnd:start] + "[...]")
		prevEnd = end
	}
	modified.WriteString(input[prevEnd:])
	return modified.String()
}
