package cli

import (
	"fmt"
	"runtime"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func generateRootCmd(cfg *config.Config) {
	// init the log formatter for logrus
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000000",
		FullTimestamp:   true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := f.File
			if len(filename) > 38 {
				filename = filename[38:]
			}
			return "", fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})

	// var (
	// 	serverFlagSet    = flag.NewFlagSet("pyroscope server", flag.ExitOnError)
	// 	agentFlagSet     = flag.NewFlagSet("pyroscope agent", flag.ExitOnError)
	// 	convertFlagSet   = flag.NewFlagSet("pyroscope convert", flag.ExitOnError)
	// 	execFlagSet      = flag.NewFlagSet("pyroscope exec", flag.ExitOnError)
	// 	connectFlagSet   = flag.NewFlagSet("pyroscope connect", flag.ExitOnError)
	// 	dbmanagerFlagSet = flag.NewFlagSet("pyroscope dbmanager", flag.ExitOnError)
	// 	rootFlagSet      = flag.NewFlagSet("pyroscope", flag.ExitOnError)
	// )

	// serverSortedFlags := PopulateFlagSet(&cfg.Server, serverFlagSet)
	// agentSortedFlags := PopulateFlagSet(&cfg.Agent, agentFlagSet, WithSkip("targets"))
	// convertSortedFlags := PopulateFlagSet(&cfg.Convert, convertFlagSet)
	// execSortedFlags := PopulateFlagSet(&cfg.Exec, execFlagSet, WithSkip("pid"))
	// connectSortedFlags := PopulateFlagSet(&cfg.Exec, connectFlagSet, WithSkip("group-name", "user-name", "no-root-drop"))
	// dbmanagerSortedFlags := PopulateFlagSet(&cfg.DbManager, dbmanagerFlagSet)
	// rootSortedFlags := PopulateFlagSet(cfg, rootFlagSet)

	// options := []ff.Option{
	// 	ff.WithConfigFileParser(parser),
	// 	ff.WithEnvVarPrefix("PYROSCOPE"),
	// 	ff.WithAllowMissingConfigFile(true),
	// 	ff.WithConfigFileFlag("config"),
	// }

	return
}
