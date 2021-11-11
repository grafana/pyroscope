package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

// admin
func newAdminCmd(cfg *config.Admin) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "admin",
		Short: "administration commands",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			fmt.Println(cfg)
			printUsageMessage(cmd)
			return nil
		}),
	}

	// admin
	cmd.AddCommand(newAdminAppCmd(cfg))

	return cmd
}

// admin app
func newAdminAppCmd(cfg *config.Admin) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "app",
		Short: "",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			fmt.Println(cfg)
			printUsageMessage(cmd)
			return nil
		}),
	}

	cmd.AddCommand(newAdminAppGetCmd(cfg))

	return cmd
}

// admin app get
func newAdminAppGetCmd(cfg *config.Admin) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "get [flags]",
		Short: "get the list of all apps",
		Long:  "get the list of all apps",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			client, err := admin.NewClient(cfg.SocketPath)
			if err != nil {
				return err
			}

			appNames, err := client.GetAppsNames()
			if err != nil {
				return err
			}

			for _, name := range appNames {
				fmt.Println(name)
			}

			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
