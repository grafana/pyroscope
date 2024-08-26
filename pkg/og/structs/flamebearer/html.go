package flamebearer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"text/template"

	"github.com/grafana/pyroscope/pkg/og/build"
)

// TODO(kolesnikovae): Refactor to ./convert?

// FlamebearerToStandaloneHTML converts and writes a flamebearer into HTML
// TODO cache template creation and whatnot?
func FlamebearerToStandaloneHTML(fb *FlamebearerProfile, dir http.FileSystem, w io.Writer) error {
	tmpl, err := getTemplate(dir, "/standalone.html")
	if err != nil {
		return fmt.Errorf("unable to get template: %w", err)
	}
	var flamegraph []byte
	flamegraph, err = json.Marshal(fb)
	if err != nil {
		return fmt.Errorf("unable to marshal flameberarer profile: %w", err)
	}

	scriptTpl, err := template.New("standalone").Parse(
		`
<script type="text/javascript">
	window.flamegraph = {{ .Flamegraph }}
	window.buildInfo = {{ .BuildInfo }};
</script>`,
	)
	if err != nil {
		return fmt.Errorf("unable to create template: %w", err)
	}

	var buffer bytes.Buffer
	err = scriptTpl.Execute(&buffer, map[string]string{
		"Flamegraph": string(flamegraph),
		"BuildInfo":  string(build.JSON()),
	})
	if err != nil {
		return fmt.Errorf("unable to execute template: %w", err)
	}

	standaloneFlamegraphRegexp := regexp.MustCompile(`(?s)<!--\s*generate-standalone-flamegraph\s*-->`)
	newContent := standaloneFlamegraphRegexp.ReplaceAll(tmpl, buffer.Bytes())
	if bytes.Equal(tmpl, newContent) {
		return fmt.Errorf("script tag could not be applied")
	}

	_, err = w.Write(newContent)
	if err != nil {
		return fmt.Errorf("failed to write html %w", err)
	}

	return nil
}

func getTemplate(dir http.FileSystem, path string) ([]byte, error) {
	f, err := dir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not find file %s: %w", path, err)
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", path, err)
	}

	return b, nil
}
