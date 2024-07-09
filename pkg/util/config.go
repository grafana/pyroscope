// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/config.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package util

import (
	"fmt"
	"reflect"
)

func stringKeyMapToInterfaceKeyMap(m map[string]interface{}) map[interface{}]interface{} {
	result := map[interface{}]interface{}{}
	for k, v := range m {
		result[k] = v
	}
	return result
}

// DiffConfig utility function that returns the diff between two config map objects
func DiffConfig(defaultConfig, actualConfig map[interface{}]interface{}) (map[interface{}]interface{}, error) {
	output := make(map[interface{}]interface{})

	for key, value := range actualConfig {

		defaultValue, ok := defaultConfig[key]
		if !ok {
			output[key] = value
			continue
		}

		switch v := value.(type) {
		case int:
			defaultV, ok := defaultValue.(int)
			if !ok || defaultV != v {
				output[key] = v
			}
		case string:
			defaultV, ok := defaultValue.(string)
			if !ok || defaultV != v {
				output[key] = v
			}
		case bool:
			defaultV, ok := defaultValue.(bool)
			if !ok || defaultV != v {
				output[key] = v
			}
		case []interface{}:
			defaultV, ok := defaultValue.([]interface{})
			if !ok || !reflect.DeepEqual(defaultV, v) {
				output[key] = v
			}
		case float64:
			defaultV, ok := defaultValue.(float64)
			if !ok || !reflect.DeepEqual(defaultV, v) {
				output[key] = v
			}
		case nil:
			if defaultValue != nil {
				output[key] = v
			}
		case map[interface{}]interface{}:
			defaultV, ok := defaultValue.(map[interface{}]interface{})
			if !ok {
				output[key] = value
			}
			diff, err := DiffConfig(defaultV, v)
			if err != nil {
				return nil, err
			}
			if len(diff) > 0 {
				output[key] = diff
			}
		case map[string]interface{}:
			defaultV, ok := defaultValue.(map[string]interface{})
			if !ok {
				output[key] = value
			}
			diff, err := DiffConfig(stringKeyMapToInterfaceKeyMap(defaultV), stringKeyMapToInterfaceKeyMap(v))
			if err != nil {
				return nil, err
			}
			if len(diff) > 0 {
				output[key] = diff
			}
		default:
			return nil, fmt.Errorf("unsupported type %T", v)
		}
	}

	return output, nil
}
