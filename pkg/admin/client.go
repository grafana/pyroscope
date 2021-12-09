package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-multierror"
)

type Client struct {
	httpClient *http.Client
}

// TODO since this is shared between client/server
// maybe we could share it?
const AppsEndpoint = "http://pyroscope/v1/apps"

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
	resp, err := c.httpClient.Get(AppsEndpoint)
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

	req, err := http.NewRequest(http.MethodDelete, AppsEndpoint, bytes.NewBuffer(marshalledPayload))
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

func checkStatusCodeOK(statusCode int) error {
	statusOK := statusCode >= 200 && statusCode < 300
	if !statusOK {
		return fmt.Errorf("Received non 2xx status code: %d", statusCode)
	}
	return nil
}
