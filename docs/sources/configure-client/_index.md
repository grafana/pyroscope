---
aliases:
  - /docs/phlare/latest/operators-guide/configure-agent/
  - /docs/phlare/latest/configure-client/
title: "Sending profiles from your application"
menuTitle: "Configure the Client"
description: ""
weight: 30
---

# Sending profiles from your application

Pyroscope is a continuous profiling database that allows you to analyze the performance of your applications. When sending profiles to Pyroscope, you can choose between two primary methods: SDK Instrumentation and Auto-Instrumentation using the Grafana agent. This document will explain these two techniques and guide you when to choose each one.

![Pyroscope agent server diagram](pyroscope-agent-server-diag.png)

## Grafana Agent (Auto-Instrumentation)

The Grafana agent is a component that runs alongside your application and periodically gathers profiling data from it. This method is suitable when you want to collect profiles from existing applications without modifying their source code. This approach is made even simpler with the eBPF profiling option that doesn't necessitate pull or push mechanisms. Here's how it works:

1. Install and configure the Grafana agent on the same machine or container where your application is running
2. The agent will periodically retrieve your application's performance profiling data, regardless of the language or technology stack your application is using
3. The captured profiles are then sent to the Pyroscope server for storage and analysis

Using the Grafana agent provides a hassle-free option, especially when dealing with multiple applications or microservices, allowing you to centralize the profiling process without changing your application's codebase.

## Pyroscope SDKs (SDK Instrumentation)

Alternatively, Pyroscope SDKs offer you the ability to instrument your application directly for more precise profiling. This mode is suitable when you wish to have complete control over the profiling process or when the application you are profiling is written in a language supported by the SDKs (e.g. Java, Python, .NET, etc.). Here's how to use Pyroscope SDKs:

1. Install the relevant Pyroscope SDK for your application's programming language (e.g. pip package, npm package, Ruby gem, etc.)
2. Instrument your application's code using the SDK to capture the necessary profiling data
3. SDK will automatically periodically push the captured profiles to the Pyroscope server for storage and analysis

By using the Pyroscope SDKs, you have the flexibility to customize the profiling process according to your application's specific requirements. You can selectively profile specific sections of code or send profiles at different intervals, depending on your needs.

## Choosing the Grafana agent or Pyroscope SDK to send profiles

The choice between using Grafana Agent (Auto-Instrumentation) or Pyroscope SDKs (SDK Instrumentation) depends on your specific use case and requirements. Here are some factors to consider when making the choice:

- Ease of setup: The Grafana agent is an ideal choice for a quick and straightforward setup without modifying your application's code. Note that eBPF profiling supports some languages (i.e. Golang) better than others, but more robust support for Python, Java, and other languages is coming soon!
- Language support: If your application is written in a language supported by the Pyroscope SDKs, and you want more control over the profiling process, using the SDKs is recommended.
- Flexibility: The Pyroscope SDKs offer greater flexibility in terms of customizing the profiling process and capturing specific sections of code with labels. If you have particular profiling needs or want to fine-tune the data collection process, the SDKs would be your best bet.

To get started choose one of the integrations below:
<table>
   <tr>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/go_pull"><img src="https://github-production-user-asset-6210df.s3.amazonaws.com/223048/257522425-48683963-91ae-4caf-8c52-ce131e25bd65.png" width="100px;" alt=""/><br />
        <b>Grafana Agent</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/go_pull/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/"><img src="https://user-images.githubusercontent.com/23323466/178160549-2d69a325-56ec-4e19-bca7-d460d400b163.png" width="100px;" alt=""/><br />
        <b>Golang</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/golang-push" title="golang-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/java/"><img src="https://user-images.githubusercontent.com/23323466/178160550-2b5a623a-0f4c-4911-923f-2c825784d45d.png" width="100px;" alt=""/><br />
        <b>Java</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/java/">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/java/rideshare" title="java-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/ebpf"><img src="https://user-images.githubusercontent.com/23323466/178160548-e974c080-808d-4c5d-be9b-c983a319b037.png" width="100px;" alt=""/><br />
        <b>eBPF</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/ebpf" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/ebpf" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/"><img src="https://user-images.githubusercontent.com/23323466/178160553-c78b8c15-99b4-43f3-a2a0-252b6c4862b1.png" width="100px;" alt=""/><br />
        <b>Python</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/python" title="python-examples">Examples</a>
      </td>
   </tr>
   <tr>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/dotnet/"><img src="https://user-images.githubusercontent.com/23323466/178160544-d2e189c6-a521-482c-a7dc-5375c1985e24.png" width="100px;" alt=""/><br />
        <b>Dotnet</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/dotnet/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/dotnet" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/"><img src="https://user-images.githubusercontent.com/23323466/178160554-b0be2bc5-8574-4881-ac4c-7977c0b2c195.png" width="100px;" alt=""/><br />
        <b>Ruby</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/ruby" title="ruby-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/"><img src="https://user-images.githubusercontent.com/23323466/178160551-a79ee6ff-a5d6-419e-89e6-39047cb08126.png" width="100px;" alt=""/><br />
        <b>NodeJS</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/nodejs/express" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/"><img src="https://user-images.githubusercontent.com/23323466/178160555-fb6aeee7-5d31-4bcb-9e3e-41e9f2f7d5b4.png" width="100px;" alt=""/><br />
        <b>Rust</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/rust/rideshare" title="examples">Examples</a>
      </td>
   </tr>
</table>


If you have more questions feel free to reach out in our Slack channel or create an issue on GitHub and the Pyroscope team will help!
