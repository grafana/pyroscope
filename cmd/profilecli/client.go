package main

import (
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	envPrefix = "PROFILECLI_"

	protocolTypeConnect = "connect"
	protocolTypeGRPC    = "grpc"
	protocolTypeGRPCWeb = "grpc-web"
)

var userAgentHeader = fmt.Sprintf("pyroscope/%s", version.Version)

type phlareClient struct {
	TenantID  string
	URL       string
	BasicAuth struct {
		Username string
		Password string
	}
	defaultTransport http.RoundTripper
	client           *http.Client
	protocol         string
}

type authRoundTripper struct {
	client *phlareClient
	next   http.RoundTripper
}

func (a *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c := a.client; c != nil {
		if c.TenantID != "" {
			req.Header.Set("X-Scope-OrgID", c.TenantID)
		}
		if c.BasicAuth.Username != "" || c.BasicAuth.Password != "" {
			req.SetBasicAuth(c.BasicAuth.Username, c.BasicAuth.Password)
		}
	}

	req.Header.Set("User-Agent", userAgentHeader)
	return a.next.RoundTrip(req)
}

func (c *phlareClient) httpClient() *http.Client {
	if c.client == nil {
		if c.defaultTransport == nil {
			c.defaultTransport = http.DefaultTransport
		}
		c.client = &http.Client{Transport: &authRoundTripper{
			client: c,
			next:   c.defaultTransport,
		}}
	}
	return c.client
}

func (c *phlareClient) protocolOption() connect.ClientOption {
	switch c.protocol {
	case protocolTypeGRPC:
		return connect.WithGRPC()
	case protocolTypeGRPCWeb:
		return connect.WithGRPCWeb()
	case protocolTypeConnect:
		return connect.WithClientOptions()
	default:
		return connect.WithClientOptions()
	}
}

type commander interface {
	Flag(name, help string) *kingpin.FlagClause
	Arg(name, help string) *kingpin.ArgClause
}

func addPhlareClient(cmd commander) *phlareClient {
	client := &phlareClient{}

	cmd.Flag("url", "URL of the profile store.").Default("http://localhost:4040").Envar(envPrefix + "URL").StringVar(&client.URL)
	cmd.Flag("tenant-id", "The tenant ID to be used for the X-Scope-OrgID header.").Default("").Envar(envPrefix + "TENANT_ID").StringVar(&client.TenantID)
	cmd.Flag("username", "The username to be used for basic auth.").Default("").Envar(envPrefix + "USERNAME").StringVar(&client.BasicAuth.Username)
	cmd.Flag("password", "The password to be used for basic auth.").Default("").Envar(envPrefix + "PASSWORD").StringVar(&client.BasicAuth.Password)
	cmd.Flag("protocol", "The protocol to be used for communicating with the server.").Default(protocolTypeConnect).EnumVar(&client.protocol,
		protocolTypeConnect, protocolTypeGRPC, protocolTypeGRPCWeb)
	return client
}
