package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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

// admin app delete
func newAdminAppDeleteCmd(cfg *config.Admin) *cobra.Command {
	vpr := newViper()
	cmd := &cobra.Command{
		Use:   "delete [flags] [app_name]",
		Short: "delete an app",
		Long:  "delete an app",
		Args:  cobra.ExactArgs(1),
		RunE: cli.CreateCmdRunFn(cfg, vpr, func(_ *cobra.Command, arg []string) error {
			appname := arg[0]
			client, err := admin.NewClient(cfg.SocketPath)
			if err != nil {
				return err
			}

			// since this is a very destructive action
			// we ask the user to type it out the app name as a form of validation
			fmt.Println(fmt.Sprintf("Are you sure you want to delete the app '%s'? This action can not be reversed.", appname))
			fmt.Println("")
			fmt.Println("Keep in mind the following:")
			fmt.Println("a) If an agent is still running, the app will be recreated.")
			fmt.Println("b) The API is idempotent, ie. if the app already does NOT exist, this command will run just fine.")
			fmt.Println("")
			fmt.Println(fmt.Sprintf("Type '%s' to confirm (without quotes).", appname))
			reader := bufio.NewReader(os.Stdin)
			text, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			trimmed := strings.TrimRight(text, "\n")
			if trimmed != appname {
				return fmt.Errorf("The app typed does not match. Want '%s' but got '%s'", appname, trimmed)
			}

			// finally delete the app
			err = client.DeleteApp(appname)
			if err != nil {
				return fmt.Errorf("failed to delete app: %w", err)
			}

			fmt.Println(fmt.Sprintf("Deleted app '%s'.", appname))
			return nil
		}),
	}

	cli.PopulateFlagSet(cfg, cmd.Flags(), vpr)
	return cmd
}
