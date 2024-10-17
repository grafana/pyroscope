<p align="center"><img alt="Pyroscope" src="https://github.com/grafana/pyroscope/assets/662636/c1fc4055-b33d-4e69-a450-9e7a7b2317bb" width="100%"/></p>


[![ci](https://github.com/grafana/pyroscope/actions/workflows/test.yml/badge.svg)](https://github.com/grafana/pyroscope/actions/workflows/test.yml)
[![JS Tests Status](https://github.com/grafana/pyroscope/workflows/JS%20Tests/badge.svg)](https://github.com/grafana/pyroscope/actions?query=workflow%3AJS%20Tests)
[![Go Report](https://goreportcard.com/badge/github.com/grafana/pyroscope)](https://goreportcard.com/report/github.com/grafana/pyroscope)
[![License: AGPLv3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fgrafana%2Fpyroscope.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fgrafana%2Fpyroscope?ref=badge_shield)
[![Latest release](https://img.shields.io/github/release/grafana/pyroscope.svg)](https://github.com/grafana/pyroscope/releases)
[![DockerHub](https://img.shields.io/docker/pulls/grafana/pyroscope.svg)](https://hub.docker.com/r/grafana/pyroscope)
[![GoDoc](https://godoc.org/github.com/grafana/pyroscope?status.svg)](https://godoc.org/github.com/grafana/pyroscope)

## 🎉 **Announcement: The new Explore Profiles UI is here!**

We are thrilled to announce the launch of the **Explore Profiles UI**, a brand-new way to explore and analyze your profiling data—now available as part of the Grafana Explore Apps suite! This new app brings you a **queryless**, **intuitive** experience for visualizing your profiling data, simplifying the entire process without the need to write complex queries.

https://github.com/user-attachments/assets/4db19ec7-86f3-4701-8f5f-9b7ffcebd49c

## What is Grafana Pyroscope?

Grafana Pyroscope is a continuous profiling platform designed to surface performance insights from your applications, helping you optimize resource usage such as CPU, memory, and I/O operations. With Pyroscope, you can both **proactively** and **reactively** address performance bottlenecks across your system.

The typical use cases are:

- **Proactive:** Reducing resource consumption, improving application performance, or preventing latency issues.
- **Reactive:** Quickly resolving incidents with line-level detail and debugging active CPU, memory, or I/O bottlenecks.

Pyroscope provides powerful tools to give you a comprehensive view of your application's behavior while allowing you to drill down into specific services for more targeted root cause analysis.

## How Does Pyroscope Work?

![deployment_diagram](https://grafana.com/media/docs/pyroscope/pyroscope_client_server_diagram_09_18_2024.png)

Pyroscope consists of three main components:
- **Pyroscope Server:** The server component that stores and processes profiling data.
- **Pyroscope SDKs(push) or Grafana alloy(pull) :** The client-side part of Pyroscope that collects profiling data from your applications and sends it to the server.
- **Explore Profiles UI:** A queryless, intuitive UI for visualizing and analyzing profiling data.

---

## [Pyroscope Live Demo](https://play.grafana.org/a/grafana-pyroscope-app/)

[![Pyroscope GIF Demo](https://github.com/user-attachments/assets/2faeb985-f2b6-4311-ad29-e318e850c025)](https://play.grafana.org/a/grafana-pyroscope-app/)


---

## **Quick Start: Run Pyroscope server locally**

### Homebrew
```sh
brew install pyroscope-io/brew/pyroscope
brew services start pyroscope
```

### Docker
```sh
docker run -it -p 4040:4040 grafana/pyroscope
```

For more documentation on how to configure Pyroscope server, see [our server documentation](https://grafana.com/docs/pyroscope/latest/configure-server/).

## **Quick Start: Run Explore Profiles UI in Grafana**

<img width="1728" alt="image" src="https://github.com/user-attachments/assets/67691443-6450-45b9-8064-f41056c88ade">

### Grafana Cloud
The app UI and server are both installed and running auomatically -- just start sending data!

### Grafana OSS
You can run the Explore profiles UI in Grafana by installing the plugin from the [Grafana Plugin Directory](https://grafana.com/grafana/plugins/grafana-pyroscope-app/) 

For more information, check out the [Explore Profiles README](https://github.com/grafana/explore-profiles)

## Documentation

For more information on how to use Pyroscope with other programming languages, install it on Linux, or use it in production environment, check out our documentation:

* [Getting Started](https://grafana.com/docs/pyroscope/latest/get-started/)
* [Deployment Guide](https://grafana.com/docs/pyroscope/latest/deploy-kubernetes/)
* [Pyroscope Architecture](https://grafana.com/docs/pyroscope/latest/reference-pyroscope-architecture/)

## Send data to server via Pyroscope agent (language specific)

For more documentation on how to add the Pyroscope agent to your code, see the [agent documentation](https://grafana.com/docs/pyroscope/latest/configure-client/) on our website or find language specific examples and documentation below:
<table>
   <tr>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/"><img src="https://user-images.githubusercontent.com/23323466/178160549-2d69a325-56ec-4e19-bca7-d460d400b163.png" width="100px;" alt=""/><br />
        <b>Golang</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/go_push/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/golang-push" title="golang-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/java/"><img src="https://user-images.githubusercontent.com/23323466/178160550-2b5a623a-0f4c-4911-923f-2c825784d45d.png" width="100px;" alt=""/><br />
        <b>Java</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/java/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/java/rideshare" title="java-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/"><img src="https://user-images.githubusercontent.com/23323466/178160553-c78b8c15-99b4-43f3-a2a0-252b6c4862b1.png" width="100px;" alt=""/><br />
        <b>Python</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/python/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/python" title="python-examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/"><img src="https://user-images.githubusercontent.com/23323466/178160554-b0be2bc5-8574-4881-ac4c-7977c0b2c195.png" width="100px;" alt=""/><br />
        <b>Ruby</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/ruby/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/ruby" title="ruby-examples">Examples</a>
      </td>
   </tr>
   <tr>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/"><img src="https://user-images.githubusercontent.com/23323466/178160551-a79ee6ff-a5d6-419e-89e6-39047cb08126.png" width="100px;" alt=""/><br />
        <b>Node.js</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/nodejs/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/nodejs/express" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/dotnet/"><img src="https://user-images.githubusercontent.com/23323466/178160544-d2e189c6-a521-482c-a7dc-5375c1985e24.png" width="100px;" alt=""/><br />
        <b>Dotnet</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/dotnet/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/dotnet" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/ebpf/"><img src="https://user-images.githubusercontent.com/23323466/178160548-e974c080-808d-4c5d-be9b-c983a319b037.png" width="100px;" alt=""/><br />
        <b>eBPF</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/grafana-agent/ebpf/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/grafana-agent-auto-instrumentation/ebpf" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/"><img src="https://user-images.githubusercontent.com/23323466/178160555-fb6aeee7-5d31-4bcb-9e3e-41e9f2f7d5b4.png" width="100px;" alt=""/><br />
        <b>Rust</b></a><br />
          <a href="https://grafana.com/docs/pyroscope/latest/configure-client/language-sdks/rust/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/grafana/pyroscope/tree/main/examples/language-sdk-instrumentation/rust/rideshare" title="examples">Examples</a>
      </td>
   </tr>
</table>

## [Supported Languages][supported languages]

Our documentation contains the most recent list of [supported languages] and also an overview over what [profiling types are supported per language][profile-types-languages].

Let us know what other integrations you want to see in [our issues](https://github.com/grafana/pyroscope/issues?q=is%3Aissue+is%3Aopen+label%3Anew-profilers) or in [our slack](https://slack.grafana.com).

[supported languages]: https://grafana.com/docs/pyroscope/latest/configure-client/
[profile-types-languages]: https://grafana.com/docs/pyroscope/latest/view-and-analyze-profile-data/profiling-types/#available-profiling-types

## Credits

Pyroscope is possible thanks to the excellent work of many people, including but not limited to:

* Brendan Gregg — inventor of Flame Graphs
* Julia Evans — creator of rbspy — sampling profiler for Ruby
* Vladimir Agafonkin — creator of flamebearer — fast flame graph renderer
* Ben Frederickson — creator of py-spy — sampling profiler for Python
* Adam Saponara — creator of phpspy — sampling profiler for PHP
* Alexei Starovoitov, Brendan Gregg, and many others who made BPF based profiling in Linux kernel possible
* Jamie Wong — creator of speedscope — interactive flame graph visualizer

## Contributing

To start contributing, check out our [Contributing Guide](docs/internal/contributing/README.md)


### Thanks to the contributors of Pyroscope!

[//]: contributor-faces
<a href="https://github.com/simonswine"><img src="https://avatars.githubusercontent.com/u/223048?v=4" title="simonswine" width="80" height="80"></a>
<a href="https://github.com/petethepig"><img src="https://avatars.githubusercontent.com/u/662636?v=4" title="petethepig" width="80" height="80"></a>
<a href="https://github.com/cyriltovena"><img src="https://avatars.githubusercontent.com/u/1053421?v=4" title="cyriltovena" width="80" height="80"></a>
<a href="https://github.com/eh-am"><img src="https://avatars.githubusercontent.com/u/6951209?v=4" title="eh-am" width="80" height="80"></a>
<a href="https://github.com/kolesnikovae"><img src="https://avatars.githubusercontent.com/u/12090599?v=4" title="kolesnikovae" width="80" height="80"></a>
<a href="https://github.com/Rperry2174"><img src="https://avatars.githubusercontent.com/u/23323466?v=4" title="Rperry2174" width="80" height="80"></a>
<a href="https://github.com/korniltsev"><img src="https://avatars.githubusercontent.com/u/331773?v=4" title="korniltsev" width="80" height="80"></a>
<a href="https://github.com/aocenas"><img src="https://avatars.githubusercontent.com/u/1014802?v=4" title="aocenas" width="80" height="80"></a>
<a href="https://github.com/dogfrogfog"><img src="https://avatars.githubusercontent.com/u/47758224?v=4" title="dogfrogfog" width="80" height="80"></a>
<a href="https://github.com/aleks-p"><img src="https://avatars.githubusercontent.com/u/8142643?v=4" title="aleks-p" width="80" height="80"></a>
<a href="https://github.com/abeaumont"><img src="https://avatars.githubusercontent.com/u/80059?v=4" title="abeaumont" width="80" height="80"></a>
<a href="https://github.com/pavelpashkovsky"><img src="https://avatars.githubusercontent.com/u/7372044?v=4" title="pavelpashkovsky" width="80" height="80"></a>
<a href="https://github.com/bryanhuhta"><img src="https://avatars.githubusercontent.com/u/32787160?v=4" title="bryanhuhta" width="80" height="80"></a>
<a href="https://github.com/Rustin170506"><img src="https://avatars.githubusercontent.com/u/29879298?v=4" title="Rustin170506" width="80" height="80"></a>
<a href="https://github.com/LouisInFlow"><img src="https://avatars.githubusercontent.com/u/84481279?v=4" title="LouisInFlow" width="80" height="80"></a>
<a href="https://github.com/darrenjaneczek"><img src="https://avatars.githubusercontent.com/u/38694490?v=4" title="darrenjaneczek" width="80" height="80"></a>
<a href="https://github.com/knylander-grafana"><img src="https://avatars.githubusercontent.com/u/104772500?v=4" title="knylander-grafana" width="80" height="80"></a>
<a href="https://github.com/jdbaldry"><img src="https://avatars.githubusercontent.com/u/4599384?v=4" title="jdbaldry" width="80" height="80"></a>
<a href="https://github.com/shaleynikov"><img src="https://avatars.githubusercontent.com/u/8720058?v=4" title="shaleynikov" width="80" height="80"></a>
<a href="https://github.com/09jvilla"><img src="https://avatars.githubusercontent.com/u/9610816?v=4" title="09jvilla" width="80" height="80"></a>
<a href="https://github.com/grafakus"><img src="https://avatars.githubusercontent.com/u/146180665?v=4" title="grafakus" width="80" height="80"></a>
<a href="https://github.com/joey-grafana"><img src="https://avatars.githubusercontent.com/u/90795735?v=4" title="joey-grafana" width="80" height="80"></a>
<a href="https://github.com/Eve832"><img src="https://avatars.githubusercontent.com/u/81647476?v=4" title="Eve832" width="80" height="80"></a>
<a href="https://github.com/iOliverNguyen"><img src="https://avatars.githubusercontent.com/u/6618620?v=4" title="iOliverNguyen" width="80" height="80"></a>
<a href="https://github.com/AdrK"><img src="https://avatars.githubusercontent.com/u/15175440?v=4" title="AdrK" width="80" height="80"></a>
<a href="https://github.com/alonlong"><img src="https://avatars.githubusercontent.com/u/3090383?v=4" title="alonlong" width="80" height="80"></a>
<a href="https://github.com/Loggy"><img src="https://avatars.githubusercontent.com/u/3171097?v=4" title="Loggy" width="80" height="80"></a>
<a href="https://github.com/RichiH"><img src="https://avatars.githubusercontent.com/u/754723?v=4" title="RichiH" width="80" height="80"></a>
<a href="https://github.com/cjsampson"><img src="https://avatars.githubusercontent.com/u/8391857?v=4" title="cjsampson" width="80" height="80"></a>
<a href="https://github.com/marcsanmi"><img src="https://avatars.githubusercontent.com/u/8235696?v=4" title="marcsanmi" width="80" height="80"></a>
<a href="https://github.com/cristiangreco"><img src="https://avatars.githubusercontent.com/u/316923?v=4" title="cristiangreco" width="80" height="80"></a>
<a href="https://github.com/BenAl1i"><img src="https://avatars.githubusercontent.com/u/96211182?v=4" title="BenAl1i" width="80" height="80"></a>
<a href="https://github.com/alsoba13"><img src="https://avatars.githubusercontent.com/u/3586560?v=4" title="alsoba13" width="80" height="80"></a>
<a href="https://github.com/robbymilo"><img src="https://avatars.githubusercontent.com/u/8106669?v=4" title="robbymilo" width="80" height="80"></a>
<a href="https://github.com/ekpatrice"><img src="https://avatars.githubusercontent.com/u/77462462?v=4" title="ekpatrice" width="80" height="80"></a>
<a href="https://github.com/ruslanpascoal2"><img src="https://avatars.githubusercontent.com/u/61955096?v=4" title="ruslanpascoal2" width="80" height="80"></a>
<a href="https://github.com/StasDachinsky"><img src="https://avatars.githubusercontent.com/u/23450818?v=4" title="StasDachinsky" width="80" height="80"></a>
<a href="https://github.com/gawicks"><img src="https://avatars.githubusercontent.com/u/1481491?v=4" title="gawicks" width="80" height="80"></a>
<a href="https://github.com/omarabid"><img src="https://avatars.githubusercontent.com/u/909237?v=4" title="omarabid" width="80" height="80"></a>
<a href="https://github.com/scottzhlin"><img src="https://avatars.githubusercontent.com/u/37504582?v=4" title="scottzhlin" width="80" height="80"></a>
<a href="https://github.com/Skemba"><img src="https://avatars.githubusercontent.com/u/8813875?v=4" title="Skemba" width="80" height="80"></a>
<a href="https://github.com/cuishuang"><img src="https://avatars.githubusercontent.com/u/15921519?v=4" title="cuishuang" width="80" height="80"></a>
<a href="https://github.com/wilfriedroset"><img src="https://avatars.githubusercontent.com/u/12611310?v=4" title="wilfriedroset" width="80" height="80"></a>
<a href="https://github.com/rajat2004"><img src="https://avatars.githubusercontent.com/u/37938604?v=4" title="rajat2004" width="80" height="80"></a>
<a href="https://github.com/nlamirault"><img src="https://avatars.githubusercontent.com/u/29233?v=4" title="nlamirault" width="80" height="80"></a>
<a href="https://github.com/cmonez"><img src="https://avatars.githubusercontent.com/u/39146411?v=4" title="cmonez" width="80" height="80"></a>
<a href="https://github.com/EgorMozheiko"><img src="https://avatars.githubusercontent.com/u/90687109?v=4" title="EgorMozheiko" width="80" height="80"></a>
<a href="https://github.com/cstyan"><img src="https://avatars.githubusercontent.com/u/3246492?v=4" title="cstyan" width="80" height="80"></a>
<a href="https://github.com/QuantumEnigmaa"><img src="https://avatars.githubusercontent.com/u/64951262?v=4" title="QuantumEnigmaa" width="80" height="80"></a>
<a href="https://github.com/Pranay0302"><img src="https://avatars.githubusercontent.com/u/55592629?v=4" title="Pranay0302" width="80" height="80"></a>
<a href="https://github.com/juliosaraiva"><img src="https://avatars.githubusercontent.com/u/6595701?v=4" title="juliosaraiva" width="80" height="80"></a>
<a href="https://github.com/clovis1122"><img src="https://avatars.githubusercontent.com/u/22270042?v=4" title="clovis1122" width="80" height="80"></a>
<a href="https://github.com/linthan"><img src="https://avatars.githubusercontent.com/u/13914829?v=4" title="linthan" width="80" height="80"></a>
<a href="https://github.com/dapirian"><img src="https://avatars.githubusercontent.com/u/3904462?v=4" title="dapirian" width="80" height="80"></a>
<a href="https://github.com/Cluas"><img src="https://avatars.githubusercontent.com/u/10056928?v=4" title="Cluas" width="80" height="80"></a>
<a href="https://github.com/bodji"><img src="https://avatars.githubusercontent.com/u/1321777?v=4" title="bodji" width="80" height="80"></a>
<a href="https://github.com/zmj64351508"><img src="https://avatars.githubusercontent.com/u/2457520?v=4" title="zmj64351508" width="80" height="80"></a>
<a href="https://github.com/ayeniblessing101"><img src="https://avatars.githubusercontent.com/u/29165344?v=4" title="ayeniblessing101" width="80" height="80"></a>
<a href="https://github.com/wusphinx"><img src="https://avatars.githubusercontent.com/u/1380777?v=4" title="wusphinx" width="80" height="80"></a>
<a href="https://github.com/futurelm"><img src="https://avatars.githubusercontent.com/u/43361929?v=4" title="futurelm" width="80" height="80"></a>
<a href="https://github.com/teckick"><img src="https://avatars.githubusercontent.com/u/10803535?v=4" title="teckick" width="80" height="80"></a>
<a href="https://github.com/chengjoey"><img src="https://avatars.githubusercontent.com/u/30427474?v=4" title="chengjoey" width="80" height="80"></a>
<a href="https://github.com/yashrsharma44"><img src="https://avatars.githubusercontent.com/u/31438680?v=4" title="yashrsharma44" width="80" height="80"></a>

[//]: contributor-faces
