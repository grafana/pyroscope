package main

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"go.yaml.in/yaml/v3"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	yamlBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var object any
	err = yaml.Unmarshal(yamlBytes, &object)
	if err != nil {
		return err
	}

	jsonBytes, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return err
	}

	_, err = os.Stdout.Write(jsonBytes)
	return err
}
