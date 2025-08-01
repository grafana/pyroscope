package main

import (
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	templates "github.com/grafana/pyroscope/examples/_templates"
	_ "github.com/grafana/pyroscope/examples/_templates/tempo"
)

func rootPath() (string, error) {
	cmd := exec.Command(
		"git",
		"rev-parse",
		"--show-toplevel",
	)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func run() error {
	rootPath, err := rootPath()
	if err != nil {
		return err
	}

	cleanUpPaths := make(map[string]struct{})

	if err := templates.Range(func(t *templates.Template) error {
		for _, path := range t.Destinations {
			for _, cleanUpPath := range t.CleanUpPaths {
				cleanUpPaths[filepath.Join(rootPath, path, cleanUpPath)] = struct{}{}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// remove all the paths in cleanUpPaths
	for path := range cleanUpPaths {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}

	if err := templates.Range(func(t *templates.Template) error {
		for _, path := range t.Destinations {
			fullPath := filepath.Join(rootPath, path)
			log.Println("Copying", t.Name, "to", fullPath)
			prefixDir := "assets"
			err := fs.WalkDir(t.Assets, prefixDir, func(path string, d fs.DirEntry, err error) error {
				fullPath := filepath.Join(fullPath, strings.TrimPrefix(path, prefixDir+"/"))
				if d.IsDir() {
					return os.MkdirAll(fullPath, 0755)
				}

				src, err := t.Assets.Open(path)
				if err != nil {
					return err
				}
				defer src.Close()
				dst, err := os.Create(fullPath)
				if err != nil {
					return err
				}
				defer dst.Close()
				_, err = io.Copy(dst, src)
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
