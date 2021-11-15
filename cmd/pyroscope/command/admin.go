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
			printUsageMessage(cmd)
			return nil
		}),
	}

	cmd.AddCommand(newAdminAppGetCmd(cfg))
	cmd.AddCommand(newAdminAppDeleteCmd(cfg))

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
			cli, err := admin.NewCLI(cfg.SocketPath)
			if err != nil {
				return err
			}

			return cli.GetAppsNames()
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}

// admin app delete
func newAdminAppDeleteCmd(cfg *config.Admin) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "delete [flags] [app_name]",
		Short: "delete an app",
		Long:  "delete an app",
		Args:  cobra.ExactArgs(1),
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, arg []string) error {
			cli, err := admin.NewCLI(cfg.SocketPath)
			if err != nil {
				return err
			}

			return cli.DeleteApp(arg[0])
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
