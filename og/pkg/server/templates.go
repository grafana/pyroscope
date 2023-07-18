package server

import (
	"fmt"
	"io"
	"net/http"
	"text/template"
)

func getTemplate(dir http.FileSystem, path string) (*template.Template, error) {
	f, err := dir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not find file %s: %q", path, err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %q", path, err)
	}

	tmpl, err := template.New(path).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("could not parse %s template: %q", path, err)
	}
	return tmpl, nil
}

func mustExecute(t *template.Template, w io.Writer, v interface{}) {
	if err := t.Execute(w, v); err != nil {
		panic(err)
	}
}
