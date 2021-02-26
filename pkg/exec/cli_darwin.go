// +build darwin

package exec

import "errors"

func performOSChecks(spyName string) error {
	if !isRoot() {
		return errors.New("on macOS you're required to run the agent with sudo")
	}
	return nil
}
