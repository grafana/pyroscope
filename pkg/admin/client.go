package admin

import (
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
