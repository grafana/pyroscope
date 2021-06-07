package cli

import (
	"io"
	"strconv"

	"gopkg.in/yaml.v2"

	"github.com/peterbourgon/ff/v3/ffyaml"
)

// parser is a parser for YAML file format that silently ignores fields that
// cannot be converted to a string, e.g.: maps and structs. Flags and their
// values are read from the key/value pairs defined in the config file.
// Undefined (skipped) flags will cause parse error unless WithIgnoreUndefined
// option is set to true.
//
// Due to the fact that ff package does not support YAML with fields of
// map/struct type, those need to be decoded separately.
func parser(r io.Reader, set func(name, value string) error) error {
	var m map[string]interface{}
	d := yaml.NewDecoder(r)
	if err := d.Decode(&m); err != nil && err != io.EOF {
		return ffyaml.ParseError{Inner: err}
	}
	for key, val := range m {
		for _, value := range valsToStrs(val) {
			if err := set(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func valsToStrs(val interface{}) []string {
	if vals, ok := val.([]interface{}); ok {
		ss := make([]string, len(vals))
		for i := range vals {
			ss[i] = valToStr(vals[i])
		}
		return ss
	}
	return []string{valToStr(val)}
}

func valToStr(val interface{}) string {
	switch v := val.(type) {
	case byte:
		return string([]byte{v})
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case uint64:
		return strconv.FormatUint(v, 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return ""
	}
}
