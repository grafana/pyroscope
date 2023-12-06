---
title: "AWS Lambda Profiling Extension"
menuTitle: "AWS Lambda Profiling Extension"
description: "Profiling AWS Lambda functions with Pyroscope"
weight: 100
---

# AWS Lambda Profiling Extension

## What is the Pyroscope Lambda Extension?
The [Pyroscope AWS Lambda Extension (github)](https://github.com/grafana/pyroscope-lambda-extension) allows profiling [lambda functions](https://aws.amazon.com/lambda/) with very little latency overhead over your existing code.

## How it works
The Pyroscope Lambda Extension runs a relay server on port `4040` in the same network namespace as the lambda function. This allows the Pyroscope clients to run as normal and send data to the relay server while adding minimal latency in the lambda handling.
![lambda_image_04-01](https://user-images.githubusercontent.com/23323466/186037668-44de7caa-6576-422a-b3f7-8416325f4a98.png)

For more information on how AWS Lambda Extensions work, check the [Building Extensions for AWS Lambda blogpost](https://aws.amazon.com/blogs/compute/building-extensions-for-aws-lambda-in-preview/).

## Basic example
### Step 1: Configure your lambda function
In order to use the extension you'll first need to [configure your Lambda function](https://docs.aws.amazon.com/lambda/latest/dg/invocation-layers.html) to use the extension.

You can find the most recent release on our [releases page](https://github.com/grafana/pyroscope-lambda-extension/releases).

### Step 2: Set up remote address 
Next, set up a remote address via environment variables where the extension will send data to:

The extension is configured via Environment Variables:

| env var                         | default                          | description                                    |
| ------------------------------- | -------------------------------- | ---------------------------------------------- |
| `PYROSCOPE_REMOTE_ADDRESS`      | `https://profiles-prod-001.grafana.net` | the pyroscope instance data will be relayed to |
| `PYROSCOPE_BASIC_AUTH_USER`     | `""`                             | HTTP Basic authentication user |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | `""`                             | HTTP Basic authentication password |
| `PYROSCOPE_AUTH_TOKEN`          | `""`                             | authorization key (token authentication)       |
| `PYROSCOPE_SELF_PROFILING`      | `false`                          | whether to profile the extension itself or not |
| `PYROSCOPE_LOG_LEVEL`           | `info`                           | `error` or `info` or `debug` or `trace`        |
| `PYROSCOPE_TIMEOUT`             | `10s`                            | http client timeout ([go duration format](https://pkg.go.dev/time#Duration))      |
| `PYROSCOPE_NUM_WORKERS`         | `5`                              | num of relay workers, pick based on the number of profile types |
| `PYROSCOPE_TENANT_ID`           | `""`                             | Pyroscope tenant ID, passed as X-Scope-OrgID http header (only needed for multi-tenancy)                                      |

### Step 3: Add the pyroscope agent to your code
Finally, add the Pyroscope sdk to the function. While this example shows configuration for a golang application, you can use the same configuration for any language.
```go
func HandleRequest(ctx context.Context) (string, error) {
	return fmt.Sprintf("Hello world!"), nil
}

func main() {
	// start the pyroscope agent before handling requests
	pyroscope.Start(pyroscope.Config{
		ApplicationName: "simple.golang.lambda", // YOUR APP NAME
		ServerAddress:   "http://localhost:4040", // MUST BE localhost:4040
	})

	lambda.Start(HandleRequest)
}
```

## What are the use cases?
Once you've completed the above steps, you'll be able to use the Pyroscope UI to analyze data surrounding your lambda function and make optimizations accordingly.
To learn more about the use case for the extension, check out the [Pyroscope AWS Lambda Extension blogpost](/blog/profile-aws-lambda-functions).

## Sending data to Pyroscope with Pyroscope AWS Lambda Extension

```bash
PYROSCOPE_REMOTE_ADDRESS="<URL>"
PYROSCOPE_BASIC_AUTH_USER="<User>"
PYROSCOPE_BASIC_AUTH_PASSWORD="<Password>"
# PYROSCOPE_TENANT_ID="<TenantID>" # Only needed in multi-tenant mode
```

To configure AWS Lambda Extension to send data to Pyroscope, replace the `<URL>` placeholder with the appropriate server URL. This could be the grafana.com Pyroscope URL or your own custom Pyroscope server URL.

If you need to send data to grafana.com, you'll have to configure HTTP Basic authentication. Replace `<User>` with your grafana.com stack user and `<Password>` with your grafana.com API key.

If your Pyroscope server has multi-tenancy enabled, you'll need to configure a tenant ID. Replace `<TenantID>` with your Pyroscope tenant ID.