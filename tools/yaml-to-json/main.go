package main

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"sigs.k8s.io/yaml"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	yamlBody, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	jsonBody, err := yaml.YAMLToJSON(yamlBody)
	if err != nil {
		return err
	}

	var jsonStruct interface{}
	if err := json.Unmarshal(jsonBody, &jsonStruct); err != nil {
		return err
	}
	jsonBody, err = json.MarshalIndent(&jsonStruct, "", "  ")
	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(jsonBody)
	return err
}
