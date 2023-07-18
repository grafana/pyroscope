package exec

import "errors"

func performOSChecks(_ string) error {
	if disableMacOSChecks {
		return nil
	}
	if !isRoot() {
		return errors.New("on macOS you're required to run the agent with sudo")
	}
	return nil
}
