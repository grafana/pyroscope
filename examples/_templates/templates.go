package templates

import "embed"

var templates []*Template

func Add(t *Template) {
	templates = append(templates, t)
}

func Range(f func(t *Template) error) error {
	for _, t := range templates {
		if err := f(t); err != nil {
			return err
		}
	}
	return nil
}

type Template struct {
	Name         string
	Description  string
	CleanUpPaths []string // Paths that should be cleared before we copy
	Destinations []string // Destination paths
	Assets       embed.FS
}
