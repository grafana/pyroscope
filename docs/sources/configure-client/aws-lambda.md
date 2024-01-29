---
title: "AWS Lambda profiling extension"
menuTitle: "AWS Lambda profiling extension"
description: "Profiling AWS Lambda functions with Pyroscope"
weight: 100
---

# AWS Lambda profiling extension

The Pyroscope AWS Lambda extension is a robust tool for profiling AWS Lambda functions, ensuring minimal latency impact. This profiling is essential for optimizing your functions.

## Why profile AWS Lambda functions?

AWS Lambda functions, while powerful and flexible, can lead to significant costs if not managed efficiently.
Serverless architectures, like AWS Lambda, can mask performance issues.
Since Lambda functions are billed based on execution time and allocated memory, code inefficiencies can lead to higher costs. Often, these costs accumulate unnoticed because of the following reasons:

* **Granular billing**: Lambda functions are billed in milliseconds, which can make small inefficiencies seem insignificant at first. However, when scaled to thousands or millions of invocations, these inefficiencies can lead to substantial costs.

* **Complex performance profile**: Lambda functions may interact with various services and resources, making it challenging to pinpoint performance bottlenecks.

* **Variable load**: The serverless nature of AWS Lambda means that functions might handle variable loads at different times, making it hard to optimize for every scenario.

Profiling Lambda functions helps identify these hidden performance bottlenecks, enabling developers to optimize their code for both performance and cost.
Effective profiling can reveal inefficient code paths, unnecessary memory usage, and areas where the execution time can be reduced.
By addressing these issues, organizations can significantly reduce their AWS bill, improve application responsiveness, and ensure a more efficient use of resources.

## Architecture

This extension runs a relay server on the same network namespace as the Lambda function, ensuring minimal added latency.

![Lambda Extension Architecture](https://user-images.githubusercontent.com/23323466/186037668-44de7caa-6576-422a-b3f7-8416325f4a98.png)

For more details, refer to the [Building Extensions for AWS Lambda blog post](https://aws.amazon.com/blogs/compute/building-extensions-for-aws-lambda-in-preview/).

## Set up the Pyroscope Lambda extension

To set up the Pyroscope Lamnda extension, you need to:

1. Configure your Lamda function
1. Set up your environment variables
1. Integrate the Pyroscope SDK

### Configure your Lambda function

Configure your Lambda function to use the extension. Find the latest release on our [releases page](https://github.com/grafana/pyroscope-lambda-extension/releases).

### Set up the environment variables

Configure the extension with the following environment variables:

| Environment Variable           | Default Value                           | Description                                  |
| ------------------------------ | --------------------------------------- | -------------------------------------------- |
| `PYROSCOPE_REMOTE_ADDRESS`     | `https://profiles-prod-001.grafana.net` | Destination for relayed Pyroscope data       |
| `PYROSCOPE_BASIC_AUTH_USER`    | `""`                                    | HTTP Basic authentication user               |
| `PYROSCOPE_BASIC_AUTH_PASSWORD`| `""`                                    | HTTP Basic authentication password           |
| `PYROSCOPE_SELF_PROFILING`     | `false`                                 | Whether to profile the extension itself      |
| `PYROSCOPE_LOG_LEVEL`          | `info`                                  | Log level (`error`, `info`, `debug`, `trace`)|
| `PYROSCOPE_TIMEOUT`            | `10s`                                   | HTTP client timeout (in Go duration format)  |
| `PYROSCOPE_NUM_WORKERS`        | `5`                                     | Number of relay workers                      |
| `PYROSCOPE_TENANT_ID`          | `""`                                    | Pyroscope tenant ID (for multi-tenancy)      |

### Integrate the Pyroscope SDK

The Pyroscope AWS Lambda extension is compatible with all existing Pyroscope SDKs. Here are some key considerations:
 - Initialize the SDK before setting up the AWS Lambda handler.
 - Ensure that the Pyroscope server address is configured to http://localhost:4040.

Note that the SDK packages are not automatically included in the extension layer. For Java, Python, Node.js, and Ruby, you must either include the SDK package in the function deployment package or add it as a Lambda layer. Refer to the detailed guide in the AWS Lambda documentation for your specific runtime for further instructions:
 - [Java](https://docs.aws.amazon.com/lambda/latest/dg/java-package.html#java-package-layers)
 - [Python](https://docs.aws.amazon.com/lambda/latest/dg/python-package.html#python-package-dependencies)
 - [Ruby](https://docs.aws.amazon.com/lambda/latest/dg/ruby-package.html#ruby-package-runtime-dependencies)
 - [Node.js](https://docs.aws.amazon.com/lambda/latest/dg/nodejs-package.html#nodejs-package-dependencies)

For a Golang Lambda function, integrate the Pyroscope SDK as follows:

```go
func HandleRequest(ctx context.Context) (string, error) {
    return "Hello world!", nil
}

func main() {
    pyroscope.Start(pyroscope.Config{
        ApplicationName: "simple.golang.lambda",
        ServerAddress:   "http://localhost:4040",
    })
    lambda.Start(HandleRequest)
}
```

Replace `simple.golang.lambda` with your application name.

## Use cases

Once set up, you can use the Pyroscope UI to analyze your Lambda function's data to facilitate performance optimizations. For more on this, visit our [Pyroscope AWS Lambda Extension blog post](http://pyroscope.io/blog/profile-aws-lambda-functions).

## Send data to Pyroscope

To configure the extension for data transmission:

```bash
PYROSCOPE_REMOTE_ADDRESS="<URL>"
PYROSCOPE_BASIC_AUTH_USER="<User>"
PYROSCOPE_BASIC_AUTH_PASSWORD="<Password>"
# PYROSCOPE_TENANT_ID="<TenantID>" # For multi-tenant mode
```

Replace placeholders accordingly. For sending data to Grafana, use your Grafana stack user and API key for authentication.
