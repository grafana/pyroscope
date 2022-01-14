package flamebearer

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
)

// FlameberarerToHTML converts and writes a flamebearer into HTML
func FlameberarerToHTML(fb *FlamebearerProfile, dir http.FileSystem, w io.Writer) error {
	tmpl, err := getTemplate(dir, "/standalone.html")
	if err != nil {
		return fmt.Errorf("unable to get template: %w", err)
	}
	var flamegraph []byte
	flamegraph, err = json.Marshal(fb)
	if err != nil {
		return fmt.Errorf("unable to marshal flameberarer profile: %w", err)
	}

	if err := tmpl.Execute(w, map[string]string{"Flamegraph": string(flamegraph)}); err != nil {
		return fmt.Errorf("unable to execupte template: %w", err)
	}
	return nil
}

func getTemplate(dir http.FileSystem, path string) (*template.Template, error) {
	f, err := dir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not find file %s: %w", path, err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", path, err)
	}

	tmpl, err := template.New(path).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("could not parse %s template: %w", path, err)
	}
	return tmpl, nil
}
