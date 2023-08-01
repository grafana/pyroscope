package public_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/grafana/pyroscope/public"
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
		{basePath: "", expected: "/"},
		{basePath: "     ", expected: "/"},
		{basePath: "/foobar/", expected: "/foobar/"},
		{basePath: "/foobar", expected: "/foobar/"},
		{basePath: "foobar", expected: "/foobar/"},
		{basePath: "   foobar   ", expected: "/foobar/"},
		{basePath: "http://localhost:8080/foobar/", expected: "http://localhost:8080/foobar/"},
		{basePath: "http://localhost:8080/foobar", expected: "http://localhost:8080/foobar/"},
	} {
		tc := tc
		t.Run(fmt.Sprintf("'%s' -> '%s'", tc.basePath, tc.expected), func(t *testing.T) {
			data, err := public.ExecuteTemplate(bin, public.Params{BasePath: tc.basePath})
			assert.NoError(t, err)
			assert.Equal(t, fmt.Sprintf(`<base href="%s" />`, tc.expected), strings.TrimSpace(string(data)))
		})
	}
}
