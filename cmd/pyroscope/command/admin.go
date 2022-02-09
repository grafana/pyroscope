package command

import (
	"fmt"
	"time"

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
	cmd.AddCommand(newAdminUserCmd(cfg))

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

	cmd.AddCommand(newAdminAppGetCmd(&cfg.AdminAppGet))
	cmd.AddCommand(newAdminAppDeleteCmd(&cfg.AdminAppDelete))

	return cmd
}

func newAdminUserCmd(cfg *config.Admin) *cobra.Command {
	vpr := newViper()

	var cmd *cobra.Command
	cmd = &cobra.Command{
		Use:   "user",
		Short: "manage users",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			printUsageMessage(cmd)
			return nil
		}),
	}

	cmd.AddCommand(newAdminPasswordResetCmd(&cfg.AdminUserPasswordReset))

	return cmd
}

// admin app get
func newAdminAppGetCmd(cfg *config.AdminAppGet) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "get [flags]",
		Short: "get the list of all apps",
		Long:  "get the list of all apps",
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, _ []string) error {
			cli, err := admin.NewCLI(cfg.SocketPath, cfg.Timeout)
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
func newAdminAppDeleteCmd(cfg *config.AdminAppDelete) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "delete [flags] [app_name]",
		Short: "delete an app",
		Long:  "delete an app",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			cli, err := admin.NewCLI(cfg.SocketPath, time.Second*2)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			appNames, err := cli.CompleteApp(toComplete)
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			return appNames, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, arg []string) error {
			cli, err := admin.NewCLI(cfg.SocketPath, cfg.Timeout)
			if err != nil {
				return err
			}

			return cli.DeleteApp(arg[0], cfg.Force)
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}

func newAdminPasswordResetCmd(cfg *config.AdminUserPasswordReset) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "reset-password [flags]",
		Short: "reset user password",
		Args:  cobra.NoArgs,
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, arg []string) error {
			ac, err := admin.NewCLI(cfg.SocketPath, cfg.Timeout)
			if err != nil {
				return err
			}
			if err = ac.ResetUserPassword(cfg.Username, cfg.Password, cfg.Enable); err != nil {
				return err
			}
			fmt.Println("Password for user", cfg.Username, "has been reset successfully.")
			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
