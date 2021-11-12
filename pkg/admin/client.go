package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	httpClient *http.Client
}

// TODO since this is shared between client/server
// maybe we could share it?
const AppsEndpoint = "http://pyroscope/v1/apps"

func NewClient(socketAddr string) (*Client, error) {
	httpClient, err := NewHTTPOverUDSClient(socketAddr)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient,
	}, nil
}

type AppNames []string

func (c *Client) GetAppsNames() (names AppNames, err error) {
	resp, err := c.httpClient.Get(AppsEndpoint)
	if err != nil {
		return names, fmt.Errorf("error making the request %w", err)
	}

	if err := checkStatusCodeOK(resp.StatusCode); err != nil {
		return names, err
	}

	err = json.NewDecoder(resp.Body).Decode(&names)
	if err != nil {
		return names, fmt.Errorf("error decoding response %w", err)
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
		return fmt.Errorf("error marshalling the payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodDelete, AppsEndpoint, bytes.NewBuffer(marshalledPayload))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making the request: %w", err)
	}

	return checkStatusCodeOK(resp.StatusCode)
}

func checkStatusCodeOK(statusCode int) error {
	statusOK := statusCode >= 200 && statusCode < 300
	if !statusOK {
		return fmt.Errorf("Received non 2xx status code: %d", statusCode)
	}
	return nil
}
