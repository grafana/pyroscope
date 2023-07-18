package public

import (
	"bytes"
	"net/url"
	"strings"
	"text/template"
)

type Params struct {
	BasePath string
}

// ExecuteTemplate executes a template using parameters
// It will transform each parameter as needed
func ExecuteTemplate(file []byte, params Params) ([]byte, error) {
	var err error
	params.BasePath, err = prepareBasePath(params.BasePath)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("").Parse(string(file))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, map[string]string{
		"BaseURL": params.BasePath,
	}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func prepareBasePath(basepath string) (string, error) {
	u, err := url.Parse(basepath)
	if err != nil {
		return "", err
	}

	u.Path = strings.TrimSpace(u.Path)

	// ideally we would use https://pkg.go.dev/net/url#JoinPath
	// but I found different results in darwin (it doesn't remove the leading slash)
	u.Path = strings.Join(
		// TODO: remove /ui/ once ui routes are moved to root
		[]string{strings.TrimRight(u.Path, "/"), "ui/"},
		"/",
	)

	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}

	return u.String(), nil
}
