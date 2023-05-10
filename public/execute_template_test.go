package public_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/grafana/phlare/public"
	"github.com/stretchr/testify/assert"
)

func TestInjectingBaseURL(t *testing.T) {
	bin, err := os.ReadFile("testdata/baseurl.html")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		basePath string
		expected string
	}{
		{basePath: "", expected: "/ui/"},
		{basePath: "     ", expected: "/ui/"},
		{basePath: "/foobar/", expected: "/foobar/ui/"},
		{basePath: "/foobar", expected: "/foobar/ui/"},
		{basePath: "foobar", expected: "/foobar/ui/"},
		{basePath: "   foobar   ", expected: "/foobar/ui/"},
		{basePath: "http://localhost:8080/foobar/", expected: "http://localhost:8080/foobar/ui/"},
		{basePath: "http://localhost:8080/foobar", expected: "http://localhost:8080/foobar/ui/"},
	} {
		tc := tc
		t.Run(fmt.Sprintf("'%s' -> '%s'", tc.basePath, tc.expected), func(t *testing.T) {
			data, err := public.ExecuteTemplate(bin, public.Params{BasePath: tc.basePath})
			assert.NoError(t, err)
			assert.Equal(t, fmt.Sprintf(`<base href="%s" />`, tc.expected), strings.TrimSpace(string(data)))
		})
	}
}
