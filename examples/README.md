# Examples

Choose a language folder to select an example for your language of choice.

# How Pyroscope works

Pyroscope identifies performance issues in your application by continuously profiling the code.

If you've never used a profiler before, then welcome!

If you are familiar with profiling and flame graphs, then you'll be happy to know that Pyroscope:
- Requires very minimal overhead
- Can store years of perf data down to 10 second granularity
- Uses a unique, inverted flame graph for increased readability


## Auto-instrumentation and language SDK instrumentation examples

Pyroscope is a continuous profiling database that allows you to analyze the performance of your applications.
When sending profiles to Pyroscope, you can choose between two primary methods: SDK Instrumentation and auto-instrumentation using Grafana Alloy.

![Pyroscope agent server diagram](https://grafana.com/media/docs/pyroscope/pyroscope_client_server_diagram_09_18_2024.png)

### About auto-instrumentation with Grafana Alloy or Agent collectors

You can send data from your application using Grafana Alloy or Grafana Agent collectors.
Both collectors support profiling with eBPF, Java, and Golang in pull mode.

For examples using auto-instrumentation with the collectors, try the `grafana-alloy-auto-instrumentation` example.

[Grafana Alloy](https://grafana.com/docs/alloy/latest/) is a vendor-neutral distribution of the OpenTelemetry (OTel) Collector.
Alloy uniquely combines the very best OSS observability signals in the community.
Grafana Alloy uses configuration file written using River.

Alloy is the recommended collector instead of Grafana Agent.
New installations should use Alloy.

Here's how it works:

1. Install and configure the collector on the same machine or container where your application is running.
2. The collector periodically retrieves your application's performance profiling data, regardless of the language or technology stack your application is using.
3. The captured profiles are then sent to the Pyroscope server for storage and analysis.

Using a collector provides a hassle-free option, especially when dealing with multiple applications or microservices, allowing you to centralize the profiling process without changing your application's codebase.

### About instrumentation with Pyroscope SDKs

Alternatively, Pyroscope SDKs offer you the ability to instrument your application directly for more precise profiling.
Use the SDKs when you want complete control over the profiling process or when the application you are profiling is written in a language supported by the SDKs (for example, Java, Python, .NET, and others).

Here's how to use Pyroscope SDKs:

1. Install the relevant Pyroscope SDK for your application's programming language (for example, pip package, npm package, Ruby gem).
2. Instrument your application's code using the SDK to capture the necessary profiling data
3. SDK automatically periodically pushes the captured profiles to the Pyroscope server for storage and analysis

By using the Pyroscope SDKs, you have the flexibility to customize the profiling process according to your application's specific requirements.
You can selectively profile specific sections of code or send profiles at different intervals, depending on your needs.

### Choose a Grafana collector or Pyroscope SDK to send profiles

You can use Grafana Alloy (recommended) or Grafana Agent (legacy) collectors for auto-instrumentation or the Pyroscope instrumentation SDKs.
The method you choose depends on your specific use case and requirements.

Here are some factors to consider when making the choice:

- Ease of setup: The Grafana Alloy/Agent collector is an ideal choice for a quick and straightforward setup without modifying your application's code. Note that eBPF profiling supports some languages (for example, Golang) better than others. More robust support for Python, Java, and other languages are in development.
- Language support: If you want more control over the profiling process and your application is written in a language supported by the Pyroscope SDKs, consider using the SDKs.
- Flexibility: The Pyroscope SDKs offer greater flexibility in terms of customizing the profiling process and capturing specific sections of code with labels. If you have particular profiling needs or want to fine-tune the data collection process, the SDKs would be your best bet.

To get started, choose one of the integrations below:
<table>
   <tr>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/go_pull"><img src="https://grafana.com/media/docs/alloy/alloy_icon.png" width="100px;" alt=""/><br />
        <b>Grafana Alloy</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/go_pull/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/grafana-alloy-auto-instrumentation" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/"><img src="https://user-images.githubusercontent.com/23323466/178160549-2d69a325-56ec-4e19-bca7-d460d400b163.png" width="100px;" alt=""/><br />
        <b>Golang</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/golang-push" title="golang-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/java/"><img src="https://user-images.githubusercontent.com/23323466/178160550-2b5a623a-0f4c-4911-923f-2c825784d45d.png" width="100px;" alt=""/><br />
        <b>Java</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/java/">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/java/rideshare" title="java-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/ebpf"><img src="https://user-images.githubusercontent.com/23323466/178160548-e974c080-808d-4c5d-be9b-c983a319b037.png" width="100px;" alt=""/><br />
        <b>eBPF</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-alloy/ebpf" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/grafana-alloy-auto-instrumentation/ebpf" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/"><img src="https://user-images.githubusercontent.com/23323466/178160553-c78b8c15-99b4-43f3-a2a0-252b6c4862b1.png" width="100px;" alt=""/><br />
        <b>Python</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/python" title="python-examples">Examples</a>
      </td>
   </tr>
   <tr>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/dotnet/"><img src="https://user-images.githubusercontent.com/23323466/178160544-d2e189c6-a521-482c-a7dc-5375c1985e24.png" width="100px;" alt=""/><br />
        <b>.NET</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/dotnet/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/dotnet" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/"><img src="https://user-images.githubusercontent.com/23323466/178160554-b0be2bc5-8574-4881-ac4c-7977c0b2c195.png" width="100px;" alt=""/><br />
        <b>Ruby</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/ruby" title="ruby-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/"><img src="https://user-images.githubusercontent.com/23323466/178160551-a79ee6ff-a5d6-419e-89e6-39047cb08126.png" width="100px;" alt=""/><br />
        <b>Node.js</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/nodejs/express" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/"><img src="https://user-images.githubusercontent.com/23323466/178160555-fb6aeee7-5d31-4bcb-9e3e-41e9f2f7d5b4.png" width="100px;" alt=""/><br />
        <b>Rust</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/rust/rideshare" title="examples">Examples</a>
      </td>
   </tr>
</table>

## Enriching profile data

You can add tags to your profiles to help correlate them with your other telemetry signals. Some common tags that are used are version, region, environment, request types, etc. You have the ability to add tags using both the SDK and the agent.

Valid tag formats may contain ASCII letters and digits, as well as underscores. It must match the regex `[a-zA-Z_][a-zA-Z0-9_]`.
In Pyroscope, a period (`.`) is not a valid character inside of tags and labels.
