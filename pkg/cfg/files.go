package cfg

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/drone/envsubst"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// JSON returns a Source that opens the supplied `.json` file and loads it.
func JSON(f *string) Source {
	return func(dst Cloneable) error {
		if f == nil {
			return nil
		}

		j, err := os.ReadFile(*f)
		if err != nil {
			return err
		}

		err = dJSON(j)(dst)
		return errors.Wrap(err, *f)
	}
}

// dJSON returns a JSON source and allows dependency injection
func dJSON(y []byte) Source {
	return func(dst Cloneable) error {
		return json.Unmarshal(y, dst)
	}
}

// YAML returns a Source that opens the supplied `.yaml` file and loads it.
// When expandEnvVars is true, variables in the supplied '.yaml\ file are expanded
// using https://pkg.go.dev/github.com/drone/envsubst?tab=overview
func YAML(f string, expandEnvVars bool) Source {
	return yamlWithKnowFields(f, expandEnvVars, true)
}

// Like YAML but ignores fields that are not known
func YAMLIgnoreUnknownFields(f string, expandEnvVars bool) Source {
	return yamlWithKnowFields(f, expandEnvVars, false)
}

func yamlWithKnowFields(f string, expandEnvVars bool, knownFields bool) Source {
	return func(dst Cloneable) error {
		y, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if expandEnvVars {
			s, err := envsubst.EvalEnv(string(y))
			if err != nil {
				return err
			}
			y = []byte(s)
		}
		err = dYAML(y, knownFields)(dst)
		return errors.Wrap(err, f)
	}
}

// dYAML returns a YAML source and allows dependency injection
func dYAML(y []byte, knownFields bool) Source {
	return func(dst Cloneable) error {
		if len(y) == 0 {
			return nil
		}
		dec := yaml.NewDecoder(bytes.NewReader(y))
		dec.KnownFields(knownFields)
		if err := dec.Decode(dst); err != nil {
			return err
		}
		return nil
	}
}

func YAMLFlag(args []string, name string) Source {
	return func(dst Cloneable) error {
		freshFlags := flag.NewFlagSet("config-file-loader", flag.ContinueOnError)

		// Ensure we register flags on a copy of the config so as to not mutate it while
		// parsing out the config file location.
		dst.Clone().RegisterFlags(freshFlags)

		freshFlags.Usage = func() { /* don't do anything by default, we will print usage ourselves, but only when requested. */ }

		if err := freshFlags.Parse(args); err != nil {
			fmt.Fprintln(freshFlags.Output(), "Run with -help to get a list of available parameters")
			if testMode {
				return err
			}
			os.Exit(2)
		}

		f := freshFlags.Lookup(name)
		if f == nil || f.Value.String() == "" {
			return nil
		}
		expandEnv := false
		expandEnvFlag := freshFlags.Lookup("config.expand-env")
		if expandEnvFlag != nil {
			expandEnv, _ = strconv.ParseBool(expandEnvFlag.Value.String()) // Can ignore error as false returned
		}

		return YAML(f.Value.String(), expandEnv)(dst)
	}
}
