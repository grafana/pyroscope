package convert

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGroups(t *testing.T) {
	t.Run("parses data correctly", func(t *testing.T) {
		r := bytes.NewReader([]byte("foo;bar 10\nfoo;baz 20\n"))
		result := []string{}
		ParseGroups(r, func(name []byte, val int) {
			result = append(result, fmt.Sprintf("%s %d", name, val))
		})
		assert.ElementsMatch(t, []string{"foo;bar 10", "foo;baz 20"}, result)
	})
}

func TestParseIndividualLines(t *testing.T) {
	t.Run("parses data correctly", func(t *testing.T) {
		r := bytes.NewReader([]byte("foo;bar\nfoo;baz\n"))
		result := []string{}
		ParseIndividualLines(r, func(name []byte, val int) {
			result = append(result, fmt.Sprintf("%s %d", name, val))
		})
		assert.ElementsMatch(t, []string{"foo;bar 1", "foo;baz 1"}, result)
	})
}
