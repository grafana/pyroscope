// +build darwin

package exec

import (
	"github.com/sirupsen/logrus"
)

func performOSChecks() {
	if !isRoot() {
		logrus.Fatal("on macOS you're required to run the agent with sudo")
	}
}
