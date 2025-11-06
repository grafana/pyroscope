package main

import (
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	envPrefix = "PROFILECLI_"

	protocolTypeConnect = "connect"
	protocolTypeGRPC    = "grpc"
	protocolTypeGRPCWeb = "grpc-web"

	acceptHeaderMimeType = "*/*"
)

var acceptHeaderClientCapabilities = []string{
	"allow-utf8-labelnames=true",
}

var userAgentHeader = fmt.Sprintf("pyroscope/%s", version.Version)

func addClientCapabilitiesHeader(r *http.Request, mime string, clientCapabilities []string) {
	missingClientCapabilities := make([]string, 0, len(clientCapabilities))
	for _, capability := range clientCapabilities {
		found := false
		// Check if any header value already contains this capability
		for _, value := range r.Header.Values("Accept") {
			if strings.Contains(value, capability) {
				found = true
				break
			}
		}

		if !found {
			missingClientCapabilities = append(missingClientCapabilities, capability)
		}
	}

	if len(missingClientCapabilities) > 0 {
		acceptHeader := mime
		acceptHeader += ";" + strings.Join(missingClientCapabilities, ";")
		r.Header.Add("Accept", acceptHeader)
	}
}

type phlareClient struct {
	TenantID    string
	URL         string
	BearerToken string
	BasicAuth   struct {
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
		} else if c.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.BearerToken)
		}
	}

	addClientCapabilitiesHeader(req, acceptHeaderMimeType, acceptHeaderClientCapabilities)
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

	cmd.Flag("url", "URL of the Pyroscope Endpoint. (Examples: https://profiles-prod-001.grafana.net for a Grafana Cloud endpoint, https://grafana.example.net/api/datasources/proxy/uid/<uid> when using the Grafana data source proxy)").Default("http://localhost:4040").Envar(envPrefix + "URL").StringVar(&client.URL)
	cmd.Flag("tenant-id", "The tenant ID to be used for the X-Scope-OrgID header.").Default("").Envar(envPrefix + "TENANT_ID").StringVar(&client.TenantID)
	cmd.Flag("token", "The bearer token to be used for communication with the server. Particularly useful when connecting to Grafana data source URLs (bearer token should be a Grafana Service Account token of the form 'glsa_[...]')").Default("").Envar(envPrefix + "TOKEN").StringVar(&client.BearerToken)
	cmd.Flag("username", "The username to be used for basic auth.").Default("").Envar(envPrefix + "USERNAME").StringVar(&client.BasicAuth.Username)
	cmd.Flag("password", "The password to be used for basic auth.").Default("").Envar(envPrefix + "PASSWORD").StringVar(&client.BasicAuth.Password)
	cmd.Flag("protocol", "The protocol to be used for communicating with the server.").Default(protocolTypeConnect).EnumVar(&client.protocol,
		protocolTypeConnect, protocolTypeGRPC, protocolTypeGRPCWeb)
	return client
}
