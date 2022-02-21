package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-multierror"
)

type Client struct {
	httpClient *http.Client
}

// TODO since this is shared between client/server
// maybe we could share it?
const (
	appsEndpoint  = "http://pyroscope/v1/apps"
	usersEndpoint = "http://pyroscope/v1/users"
)

var (
	ErrHTTPClientCreation = errors.New("failed to create http over uds client")
	ErrMakingRequest      = errors.New("failed to make a request")
	ErrStatusCodeNotOK    = errors.New("failed to get a response with a valid status code")
	ErrDecodingResponse   = errors.New("error while decoding a (assumed) json response")
	ErrMarshalingPayload  = errors.New("error while marshalling the payload")
)

func NewClient(socketAddr string, timeout time.Duration) (*Client, error) {
	httpClient, err := NewHTTPOverUDSClient(socketAddr, WithTimeout(timeout))

	if err != nil {
		return nil, multierror.Append(ErrHTTPClientCreation, err)
	}

	return &Client{
		httpClient,
	}, nil
}

type AppNames []string

func (c *Client) GetAppsNames() (names AppNames, err error) {
	resp, err := c.httpClient.Get(appsEndpoint)
	if err != nil {
		return names, multierror.Append(ErrMakingRequest, err)
	}

	if err := checkStatusCodeOK(resp.StatusCode); err != nil {
		return names, multierror.Append(ErrStatusCodeNotOK, err)
	}

	err = json.NewDecoder(resp.Body).Decode(&names)
	if err != nil {
		return names, multierror.Append(ErrDecodingResponse, err)
	}

	return names, nil
}

func (c *Client) DeleteApp(name string) (err error) {
	// we are kinda robbing here
	// since the server and client are defined in the same package
	payload := DeleteAppInput{
		Name: name,
	}

	marshalledPayload, err := json.Marshal(payload)
	if err != nil {
		return multierror.Append(ErrMarshalingPayload, err)
	}

	req, err := http.NewRequest(http.MethodDelete, appsEndpoint, bytes.NewBuffer(marshalledPayload))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return multierror.Append(ErrMakingRequest, err)
	}

	err = checkStatusCodeOK(resp.StatusCode)
	if err != nil {
		return multierror.Append(ErrStatusCodeNotOK, err)
	}
	return nil
}

func (c *Client) ResetUserPassword(username, password string, enable bool) error {
	reqBody := UpdateUserRequest{Password: &password}
	if enable {
		reqBody.SetIsDisabled(false)
	}

	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(reqBody); err != nil {
		return multierror.Append(ErrMarshalingPayload, err)
	}

	req, err := http.NewRequest(http.MethodPatch, usersEndpoint+"/"+username, &b)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if v, err := io.ReadAll(resp.Body); err == nil {
		return fmt.Errorf(string(v))
	}
	return fmt.Errorf("received non 2xx status code: %d", resp.StatusCode)
}

func checkStatusCodeOK(statusCode int) error {
	statusOK := statusCode >= 200 && statusCode < 300
	if !statusOK {
		return fmt.Errorf("Received non 2xx status code: %d", statusCode)
	}
	return nil
}
