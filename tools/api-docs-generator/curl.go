package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/getkin/kin-openapi/openapi3"
)

type shellCmd string

func (c shellCmd) MarshalJSON() ([]byte, error) {
	return json.Marshal(`#` + string(c) + `#`)
}

type exampleCurl struct {
	sc *openapi3.Schema
}

func newExampleCurl(sc *openapi3.Schema) *exampleCurl {
	return &exampleCurl{sc: sc}
}

func (e *exampleCurl) name() string {
	return "curl"
}

func (e *exampleCurl) render(sb io.Writer, params *exampleParams) {
	body := map[string]any{}
	collectParameters(e.sc, "", func(prefix string, name string, schema *openapi3.Schema) {
		exStr, exValue := getExample(schema)
		if exStr == "" {
			return
		}

		ex, ok := exampleParameters[exStr]
		if !ok || ex.Curl == nil {
			setBody(body, prefix, name, exValue)
			return
		}

		setBody(body, prefix, name, ex.Curl)
	})

	addLabelsToSeries(body, "__name__", "process_cpu")
	bodyJson, err := json.MarshalIndent(&body, "    ", "  ")
	if err != nil {
		panic(err)
	}

	// convert commands so they are run in bash
	bodyJson = bytes.ReplaceAll(bodyJson, []byte{'"', '#', '\\', '"'}, []byte{'"', '\''})
	bodyJson = bytes.ReplaceAll(bodyJson, []byte{'\\', '"', '#', '"'}, []byte{'\'', '"'})
	bodyJson = bytes.ReplaceAll(bodyJson, []byte{'#', '"'}, []byte{'\''})
	bodyJson = bytes.ReplaceAll(bodyJson, []byte{'"', '#'}, []byte{'\''})

	fmt.Fprintln(sb, "curl \\")
	fmt.Fprintln(sb, `  -H "Content-Type: application/json" \`)
	fmt.Fprint(sb, "  -d '")
	fmt.Fprint(sb, string(bodyJson))
	fmt.Fprintln(sb, "' \\")
	fmt.Fprintf(sb, "  %s\n", params.url)

}
