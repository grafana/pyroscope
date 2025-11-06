package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/getkin/kin-openapi/openapi3"
)

type pythonExpr string

func (e pythonExpr) MarshalJSON() ([]byte, error) { //nolint:unparam
	return []byte(`"#` + string(e) + `#"`), nil
}

type examplePython struct {
	sc *openapi3.Schema
}

func newExamplePython(sc *openapi3.Schema) *examplePython {
	return &examplePython{sc: sc}
}

func (e *examplePython) name() string {
	return "python"
}

func (e *examplePython) render(sb io.Writer, params *exampleParams) {
	body := map[string]any{}
	collectParameters(e.sc, "", func(prefix string, name string, schema *openapi3.Schema) {
		exStr, exValue := getExample(schema)
		if exStr == "" {
			return
		}

		ex, ok := exampleParameters[exStr]
		if !ok || ex.Python == nil {
			setBody(body, prefix, name, exValue)
			return
		}

		setBody(body, prefix, name, ex.Python)
	})

	addLabelsToSeries(body, "__name__", "process_cpu")
	bodyJson, err := json.MarshalIndent(&body, "  ", "  ")
	if err != nil {
		panic(err)
	}
	// convert commands so they are run in python
	bodyJson = bytes.ReplaceAll(bodyJson, []byte{'#', '"'}, []byte{})
	bodyJson = bytes.ReplaceAll(bodyJson, []byte{'"', '#'}, []byte{})

	fmt.Fprintln(sb, "import requests")
	if bytes.Contains(bodyJson, []byte("datetime")) {
		fmt.Fprintln(sb, "import datetime")
	}
	if bytes.Contains(bodyJson, []byte("base64")) {
		fmt.Fprintln(sb, "import base64")
	}
	fmt.Fprintf(sb, "body = %s\n", string(bodyJson))
	fmt.Fprintf(sb, "url = '%s'\n", params.url)
	fmt.Fprintln(sb, "resp = requests.post(url, json=body)")
	fmt.Fprintln(sb, "print(resp)")
	fmt.Fprintln(sb, "print(resp.content)")
}
