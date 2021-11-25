package exec

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/util/names"
)

// used in tests
var (
	disableMacOSChecks bool
	disableLinuxChecks bool
)

type UnsupportedSpyError struct {
	Args       []string
	Subcommand string
}

func (e UnsupportedSpyError) Error() string {
	supportedSpies := spy.SupportedExecSpies()
	suggestedCommand := fmt.Sprintf("pyroscope %s -spy-name %s %s", e.Subcommand, supportedSpies[0], strings.Join(e.Args, " "))
	return fmt.Sprintf(
		"could not automatically find a spy for program \"%s\". Pass spy name via %s argument, for example: \n"+
			"  %s\n\nAvailable spies are: %s\nIf you believe this is a mistake, please submit an issue at %s",
		path.Base(e.Args[0]),
		color.YellowString("-spy-name"),
		color.YellowString(suggestedCommand),
		strings.Join(supportedSpies, ","),
		color.GreenString("https://github.com/pyroscope-io/pyroscope/issues"),
	)
}

func NewLogger(logLevel string, noLogging bool) *logrus.Logger {
	level := logrus.PanicLevel
	if l, err := logrus.ParseLevel(logLevel); err == nil && !noLogging {
		level = l
	}
	logger := logrus.StandardLogger()
	logger.SetLevel(level)
	if level != logrus.PanicLevel {
		logger.Info("to disable logging from pyroscope, specify " + color.YellowString("-no-logging") + " flag")
	}
	// TODO(abeaumont): fix logger configuration
	return logger
}

func CheckApplicationName(logger *logrus.Logger, applicationName string, spyName string, args []string) string {
	if applicationName == "" {
		logger.Infof("we recommend specifying application name via %s flag or env variable %s",
			color.YellowString("-application-name"), color.YellowString("PYROSCOPE_APPLICATION_NAME"))
		applicationName = spyName + "." + names.GetRandomName(generateSeed(args))
		logger.Infof("for now we chose the name for you and it's \"%s\"", color.GreenString(applicationName))
	}
	return applicationName
}

func PerformChecks(spyName string) error {
	if spyName == types.GoSpy {
		return fmt.Errorf("gospy can not profile other processes. See our documentation on using gospy: %s", color.GreenString("https://pyroscope.io/docs/"))
	}

	err := performOSChecks(spyName)
	if err != nil {
		return err
	}

	if !stringsContains(spy.SupportedSpies, spyName) {
		supportedSpies := spy.SupportedExecSpies()
		return fmt.Errorf(
			"spy \"%s\" is not supported. Available spies are: %s",
			color.GreenString(spyName),
			strings.Join(supportedSpies, ","),
		)
	}

	return nil
}

func stringsContains(arr []string, element string) bool {
	for _, v := range arr {
		if v == element {
			return true
		}
	}
	return false
}

func generateSeed(args []string) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "<unknown>"
	}
	return cwd + "|" + strings.Join(args, "&")
}
