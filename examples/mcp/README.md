# Using Grafana MCP server with Pyroscope

This example demonstrates how to use the Grafana MCP server with Pyroscope.

## Quickstart

Before running any code, download the latest Grafana MCP server. For a complete
list of all possible installation options, see https://github.com/grafana/mcp-grafana?tab=readme-ov-file#usage.
To quickly get started, it's easiest to install the server via `go install`.

```shell
go install github.com/grafana/mcp-grafana/cmd/mcp-grafana@latest
```

Verify the installation worked by running

```shell
mcp-grafana -h
```

To run the example, first copy the `.env.template` file to a `.env` file.

```shell
cp .env.template .env
```

Then populate the `.env` file with the relevant tokens.

Next, start the services with Docker Compose.

```shell
docker compose up
```

This will start a Grafana instance running at http://localhost:3000 and a
Pyroscope instance running at http://localhost:4040.

We need to get an API key for the local Grafana instance. To do this, follow
these steps:

1. Open http://localhost:3000/org/serviceaccounts
1. Create a service account with a "Admin" role
1. Click "Add service account token"
1. Generate and copy the token
1. Save the copied token for the next steps.

Finally, configure your client to use the MCP server. For example, a Claude
configuration would look like this

```json
{
  "mcpServers": {
    "grafana": {
      "command": "mcp-grafana",
      "args": [],
      "env": {
        "GRAFANA_URL": "http://localhost:3000",
        "GRAFANA_API_KEY": "<your service account token>"
      }
    }
  }
}
```

Once you saved your client configuration file, you can begin using your LLM to
fetch data from Grafana. Start with asking something like:

> Can you list all the data sources in my Grafana instance?

If everything is set up correctly, the LLM should identify two data sources:
Prometheus and Pyroscope. From here, you can ask for information about Pyroscope
like:

> Can you list all the service names in Pyroscope?

Happy prompting!
