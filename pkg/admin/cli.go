package admin

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type CLIError struct{ err error }

func (e CLIError) Error() string {
	if errors.Is(e.err, ErrMakingRequest) {
		return fmt.Sprintf(`failed to contact the admin socket server. 
this may happen if
a) pyroscope server is not running
b) the socket path is incorrect
c) admin features are not enabled, in that case check the server flags

%v`, e.err)
	}

	return fmt.Sprintf("%v", e.err)
}

type CLI struct {
	client *Client
}

func NewCLI(socketPath string, timeout time.Duration) (*CLI, error) {
	client, err := NewClient(socketPath, timeout)
	if err != nil {
		return nil, err
	}

	return &CLI{
		client,
	}, nil
}

// GetAppsNames returns the list of all apps
func (c *CLI) GetAppsNames() error {
	appNames, err := c.client.GetAppsNames()
	if err != nil {
		return CLIError{err}
	}

	for _, name := range appNames {
		fmt.Println(name)
	}

	return nil
}

// DeleteApp deletes an app if a matching app exists
func (c *CLI) DeleteApp(appname string, skipVerification bool) error {
	if !skipVerification {
		// since this is a very destructive action
		// we ask the user to type it out the app name as a form of validation
		fmt.Println(fmt.Sprintf("Are you sure you want to delete the app '%s'? This action can not be reversed.", appname))
		fmt.Println("")
		fmt.Println("Keep in mind the following:")
		fmt.Println("a) This command may take a while.")
		fmt.Println("b) While it's running, it may lock the database for writes.")
		fmt.Println("c) If an agent is still running, the app will be recreated.")
		fmt.Println("d) The API is idempotent, ie. if the app already does NOT exist, this command will run just fine.")
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
	}

	// finally delete the app
	err := c.client.DeleteApp(appname)
	if err != nil {
		return CLIError{err}
	}

	fmt.Println(fmt.Sprintf("Deleted app '%s'.", appname))
	return nil
}

func (c *CLI) ResetUserPassword(username, password string, enable bool) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password are required")
	}
	return c.client.ResetUserPassword(username, password, enable)
}

// CompleteApp returns the list of apps
// it's meant for cobra's autocompletion
// TODO use the parameter for fuzzy search?
func (c *CLI) CompleteApp(_ string) (appNames []string, err error) {
	appNames, err = c.client.GetAppsNames()

	if err != nil {
		return nil, err
	}

	return appNames, err
}
