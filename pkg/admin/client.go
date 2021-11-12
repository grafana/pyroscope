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
	// TODO: retrieve the route from somewhere else
	resp, err := c.httpClient.Get("http://pyroscope/v1/apps")
	if err != nil {
		return names, fmt.Errorf("error making the request %w", err)
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
		return fmt.Errorf("error marshalling the payload")
	}

	req, err := http.NewRequest(http.MethodDelete, "http://pyroscope/v1/apps", bytes.NewBuffer(marshalledPayload))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	_, err = c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making the request: %w", err)
	}

	// TODO
	// what if this returns something different than 500?

	return nil
}
