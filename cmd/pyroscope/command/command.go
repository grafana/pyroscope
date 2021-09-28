package command

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/spf13/viper"
)

func newViper() *viper.Viper {
	return cli.NewViper("PYROSCOPE")
}
