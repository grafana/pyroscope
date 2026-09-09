package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

func main() {
	outputPath := flag.CommandLine.String("output.path", "./operations/monitoring/", "Provide the output path for the code generation. Warning: All existing files will be overwritten.")
	dashboardsPath := flag.CommandLine.String("output.dashboards.path", "", "Override dashboard output directory (defaults to <output.path>/dashboards/).")
	flag.Parse()

	if err := run(*outputPath, *dashboardsPath); err != nil {
		log.Fatal(err)
	}
}

func outputRules(outputPath string, fileName string, body []byte) error {
	if fileName == "prometheus.yaml" {
		return nil
	}
	path := filepath.Join(outputPath, "rules", fileName)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, body, 0644)
}

func outputDashboards(dashboardsDir string, fileName string, body []byte) error {
	path := filepath.Join(dashboardsDir, fileName)

	if !strings.HasSuffix(path, ".json") {
		return nil
	}

	data := map[string]any{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("error Unmarshalling body of '%s': %w", fileName, err)
	}

	bodyFormatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling body of '%s': %w", fileName, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, bodyFormatted, 0644)
}

func run(outputPath, dashboardsPath string) error {
	if dashboardsPath == "" {
		dashboardsPath = filepath.Join(outputPath, "dashboards")
	}

	d := yaml.NewDecoder(os.Stdin)
	obj := map[string]any{}

	for {
		err := d.Decode(&obj)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		metaIntf, ok := obj["metadata"]
		if !ok {
			return errors.New("metadata not found")
		}

		meta, ok := metaIntf.(map[string]any)
		if !ok {
			return fmt.Errorf("metadata not object: %T", metaIntf)
		}

		nameIntf, ok := meta["name"]
		if !ok {
			return errors.New("name not found")
		}

		name, ok := nameIntf.(string)
		if !ok {
			return errors.New("name not string")
		}

		var handle func(string, string, []byte) error

		switch name {
		case "pyroscope-monitoring-dashboards":
			handle = func(_, file string, body []byte) error {
				return outputDashboards(dashboardsPath, file, body)
			}
		case "pyroscope-monitoring-rules":
			handle = outputRules
		default:
			continue
		}

		dataIntf, ok := obj["data"]
		if !ok {
			return errors.New("data not found")
		}

		data, ok := dataIntf.(map[string]any)
		if !ok {
			return fmt.Errorf("data not object: %T", dataIntf)
		}

		for key, bodyIntf := range data {
			body, ok := bodyIntf.(string)
			if !ok {
				continue
			}
			if err := handle(outputPath, key, []byte(body)); err != nil {
				return err
			}
		}
	}
}
