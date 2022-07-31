
<p align="center"><img alt="Pyroscope" src="https://user-images.githubusercontent.com/662636/105129037-11334180-5a99-11eb-8951-1d4aaaed50de.png" width="500px"/></p>



[![Go Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/Go%20Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3AGo%20Tests)
[![JS Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/JS%20Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3AJS%20Tests)
[![Go Report](https://goreportcard.com/badge/github.com/pyroscope-io/pyroscope)](https://goreportcard.com/report/github.com/pyroscope-io/pyroscope)
[![Apache 2 License](https://img.shields.io/badge/license-Apache%202-blue.svg)](LICENSE)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fpyroscope-io%2Fpyroscope.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fpyroscope-io%2Fpyroscope?ref=badge_shield)
[![Latest release](https://img.shields.io/github/release/pyroscope-io/pyroscope.svg)](https://github.com/pyroscope-io/pyroscope/releases)
[![DockerHub](https://img.shields.io/docker/pulls/pyroscope/pyroscope.svg)](https://hub.docker.com/r/pyroscope/pyroscope)
[![GoDoc](https://godoc.org/github.com/pyroscope-io/pyroscope?status.svg)](https://godoc.org/github.com/pyroscope-io/pyroscope)

<h2>
  <a href="https://pyroscope.io/">Website</a>
  <span> • </span>
  <a href="https://pyroscope.io/docs">Docs</a>
  <span> • </span>
  <a href="https://demo.pyroscope.io/">Demo</a>
  <span> • </span>
  <a href="/examples">Examples</a>
  <span> • </span>
  <a href="https://pyroscope.io/slack">Slack</a>
</h2>

#### _Read this in other languages._
<kbd>[<img title="中文 (Simplified)" alt="中文 (Simplified)" src="https://cdn.staticaly.com/gh/hjnilsson/country-flags/master/svg/cn.svg" width="22">](translations/README.ch.md)</kbd>

### What is Pyroscope?

Pyroscope is an open source continuous profiling platform. It will help you:
* Find performance issues and bottlenecks in your code
* Use high-cardinality tags/labels to analyze your application
* Resolve issues with high CPU utilization
* Track down memory leaks
* Understand the call tree of your application
* Auto-instrument your code to link profiling data to traces


## 🔥 [Pyroscope Live Demo](https://demo.pyroscope.io/?name=hotrod.python.frontend%7B%7D) 🔥

[![Pyroscope GIF Demo](https://user-images.githubusercontent.com/23323466/143324845-16ff72df-231e-412d-bd0a-38ef2e09cba8.gif)](https://demo.pyroscope.io/)

## Features

* Minimal CPU overhead
* Efficient compression, low disk space requirements
* Can handle high-cardinality tags/labels
* Calculate the performance "diff" between various tags/labels and time periods
* Can store years of profiling data from multiple applications
* Advanced analysis UI

## Add Pyroscope Server locally in 2 steps:

Pyroscope supports all major architectures and is very easy to install. For example, here is how you install on a mac:
```shell
# install pyroscope
brew install pyroscope-io/brew/pyroscope

# start pyroscope server:
pyroscope server
```
## Send data to server via Pyroscope agent (language specific)

For more documentation on how to add the Pyroscope agent to your code, see the [agent documentation](https://pyroscope.io/docs/agent-overview) on our website or find language specific examples and documentation below:
<table>
   <tr>
      <td align="center"><a href="https://pyroscope.io/docs/golang"><img src="https://user-images.githubusercontent.com/23323466/178160549-2d69a325-56ec-4e19-bca7-d460d400b163.png" width="100px;" alt=""/><br />
        <b>Golang</b></a><br />
          <a href="https://pyroscope.io/docs/golang" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/golang-push" title="golang-examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/java"><img src="https://user-images.githubusercontent.com/23323466/178160550-2b5a623a-0f4c-4911-923f-2c825784d45d.png" width="100px;" alt=""/><br />
        <b>Java</b></a><br />
          <a href="https://pyroscope.io/docs/java" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/java-jfr/rideshare" title="java-examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/python"><img src="https://user-images.githubusercontent.com/23323466/178160553-c78b8c15-99b4-43f3-a2a0-252b6c4862b1.png" width="100px;" alt=""/><br />
        <b>Python</b></a><br />
          <a href="https://pyroscope.io/docs/python" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/python" title="python-examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/ruby"><img src="https://user-images.githubusercontent.com/23323466/178160554-b0be2bc5-8574-4881-ac4c-7977c0b2c195.png" width="100px;" alt=""/><br />
        <b>Ruby</b></a><br />
          <a href="https://pyroscope.io/docs/ruby" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/ruby" title="ruby-examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/rust"><img src="https://user-images.githubusercontent.com/23323466/178160555-fb6aeee7-5d31-4bcb-9e3e-41e9f2f7d5b4.png" width="100px;" alt=""/><br />
        <b>Rust</b></a><br />
          <a href="https://pyroscope.io/docs/rust" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/rust/rideshare" title="examples">Examples</a>
      </td>
   </tr>
   <tr>
      <td align="center"><a href="https://pyroscope.io/docs/nodejs"><img src="https://user-images.githubusercontent.com/23323466/178160551-a79ee6ff-a5d6-419e-89e6-39047cb08126.png" width="100px;" alt=""/><br />
        <b>NodeJS</b></a><br />
          <a href="https://pyroscope.io/docs/nodejs" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/nodejs/express" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/dotnet"><img src="https://user-images.githubusercontent.com/23323466/178160544-d2e189c6-a521-482c-a7dc-5375c1985e24.png" width="100px;" alt=""/><br />
        <b>Dotnet</b></a><br />
          <a href="https://pyroscope.io/docs/dotnet" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/dotnet" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/ebpf"><img src="https://user-images.githubusercontent.com/23323466/178160548-e974c080-808d-4c5d-be9b-c983a319b037.png" width="100px;" alt=""/><br />
        <b>eBPF</b></a><br />
          <a href="https://pyroscope.io/docs/ebpf" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/ebpf" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/php"><img src="https://user-images.githubusercontent.com/23323466/178160552-7aabf63a-b129-404d-8c62-16dedfefe32c.png" width="100px;" alt=""/><br />
        <b>PHP</b></a><br />
          <a href="https://pyroscope.io/docs/php" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/php" title="examples">Examples</a>
      </td>
      <td align="center"><a href="https://pyroscope.io/docs/grafana-plugins/"><img src="https://user-images.githubusercontent.com/23323466/178341477-c4ad2445-c90e-4ef9-b7f9-b6b3cf615e33.png" width="100px;" alt=""/><br />
        <b>Grafana</b></a><br />
          <a href="https://pyroscope.io/docs/grafana-plugins/" title="Documentation">Documentation</a><br />
          <a href="https://github.com/pyroscope-io/pyroscope/tree/main/examples/grafana-integration" title="examples">Examples</a>
      </td>
   </tr>
</table>

## Deployment Diagram

![agent_server_diagram_11-01](https://user-images.githubusercontent.com/23323466/178165230-a94e1ee2-9725-4752-97ff-542158d1b703.svg)

## Third-Party Integrations

Pyroscope also supports several third-party integrations notably:
- [Grafana Plugin](https://github.com/pyroscope-io/pyroscope/tree/main/examples/grafana-integration)
- [Jaeger UI](https://github.com/pyroscope-io/jaeger-ui)
- [OTel Golang (tracing)](https://github.com/pyroscope-io/otel-profiling-go)
- [AWS Lambda Extension](https://pyroscope.io/docs/aws-lambda)

## Documentation

For more information on how to use Pyroscope with other programming languages, install it on Linux, or use it in production environment, check out our documentation:

* [Public Roadmap](https://github.com/pyroscope-io/pyroscope/projects/1)
* [Getting Started](https://pyroscope.io/docs/)
* [Deployment Guide](https://pyroscope.io/docs/deployment)
* [Developer Guide](https://pyroscope.io/docs/developer-guide)


## Downloads

You can download the latest version of pyroscope for macOS, linux and Docker from our [Downloads page](https://pyroscope.io/downloads/).

## Supported Integrations

* [x] Go (via `pprof`)
* [x] Python (via `py-spy`)
* [x] Ruby (via `rbspy`)
* [x] Linux eBPF (via `profile.py` from `bcc-tools`)
* [x] Java (via `async-profiler`)
* [x] Rust (via `pprof-rs`)
* [x] .NET (via `dotnet trace`)
* [x] PHP (via `phpspy`)
* [x] Node

Let us know what other integrations you want to see in [our issues](https://github.com/pyroscope-io/pyroscope/issues?q=is%3Aissue+is%3Aopen+label%3Anew-profilers) or in [our slack](https://pyroscope.io/slack).

## Credits

Pyroscope is possible thanks to the excellent work of many people, including but not limited to:

* Brendan Gregg — inventor of Flame Graphs
* Julia Evans — creator of rbspy — sampling profiler for Ruby
* Vladimir Agafonkin — creator of flamebearer — fast flamegraph renderer
* Ben Frederickson — creator of py-spy — sampling profiler for Python
* Adam Saponara — creator of phpspy — sampling profiler for PHP
* Alexei Starovoitov, Brendan Gregg, and many others who made BPF based profiling in Linux kernel possible

## Contributing

To start contributing, check out our [Contributing Guide](CONTRIBUTING.md)


### Thanks to the contributors of Pyroscope!

[//]: contributor-faces
<a href="https://github.com/petethepig"><img src="https://avatars.githubusercontent.com/u/662636?v=4" title="petethepig" width="80" height="80"></a>
<a href="https://github.com/eh-am"><img src="https://avatars.githubusercontent.com/u/6951209?v=4" title="eh-am" width="80" height="80"></a>
<a href="https://github.com/Rperry2174"><img src="https://avatars.githubusercontent.com/u/23323466?v=4" title="Rperry2174" width="80" height="80"></a>
<a href="https://github.com/kolesnikovae"><img src="https://avatars.githubusercontent.com/u/12090599?v=4" title="kolesnikovae" width="80" height="80"></a>
<a href="https://github.com/abeaumont"><img src="https://avatars.githubusercontent.com/u/80059?v=4" title="abeaumont" width="80" height="80"></a>
<a href="https://github.com/LouisInFlow"><img src="https://avatars.githubusercontent.com/u/84481279?v=4" title="LouisInFlow" width="80" height="80"></a>
<a href="https://github.com/pavelpashkovsky"><img src="https://avatars.githubusercontent.com/u/7372044?v=4" title="pavelpashkovsky" width="80" height="80"></a>
<a href="https://github.com/shaleynikov"><img src="https://avatars.githubusercontent.com/u/8720058?v=4" title="shaleynikov" width="80" height="80"></a>
<a href="https://github.com/korniltsev"><img src="https://avatars.githubusercontent.com/u/331773?v=4" title="korniltsev" width="80" height="80"></a>
<a href="https://github.com/dogfrogfog"><img src="https://avatars.githubusercontent.com/u/47758224?v=4" title="dogfrogfog" width="80" height="80"></a>
<a href="https://github.com/iOliverN"><img src="https://avatars.githubusercontent.com/u/6618620?v=4" title="iOliverN" width="80" height="80"></a>
<a href="https://github.com/AdrK"><img src="https://avatars.githubusercontent.com/u/15175440?v=4" title="AdrK" width="80" height="80"></a>
<a href="https://github.com/alonlong"><img src="https://avatars.githubusercontent.com/u/3090383?v=4" title="alonlong" width="80" height="80"></a>
<a href="https://github.com/Loggy"><img src="https://avatars.githubusercontent.com/u/3171097?v=4" title="Loggy" width="80" height="80"></a>
<a href="https://github.com/cjsampson"><img src="https://avatars.githubusercontent.com/u/8391857?v=4" title="cjsampson" width="80" height="80"></a>
<a href="https://github.com/ekpatrice"><img src="https://avatars.githubusercontent.com/u/77462462?v=4" title="ekpatrice" width="80" height="80"></a>
<a href="https://github.com/ruslanpascoal2"><img src="https://avatars.githubusercontent.com/u/61955096?v=4" title="ruslanpascoal2" width="80" height="80"></a>
<a href="https://github.com/gawicks"><img src="https://avatars.githubusercontent.com/u/1481491?v=4" title="gawicks" width="80" height="80"></a>
<a href="https://github.com/omarabid"><img src="https://avatars.githubusercontent.com/u/909237?v=4" title="omarabid" width="80" height="80"></a>
<a href="https://github.com/EgorMozheiko"><img src="https://avatars.githubusercontent.com/u/90687109?v=4" title="EgorMozheiko" width="80" height="80"></a>
<a href="https://github.com/cmonez"><img src="https://avatars.githubusercontent.com/u/39146411?v=4" title="cmonez" width="80" height="80"></a>
<a href="https://github.com/rajat2004"><img src="https://avatars.githubusercontent.com/u/37938604?v=4" title="rajat2004" width="80" height="80"></a>
<a href="https://github.com/Skemba"><img src="https://avatars.githubusercontent.com/u/8813875?v=4" title="Skemba" width="80" height="80"></a>
<a href="https://github.com/Cluas"><img src="https://avatars.githubusercontent.com/u/10056928?v=4" title="Cluas" width="80" height="80"></a>
<a href="https://github.com/linthan"><img src="https://avatars.githubusercontent.com/u/13914829?v=4" title="linthan" width="80" height="80"></a>
<a href="https://github.com/clovis1122"><img src="https://avatars.githubusercontent.com/u/22270042?v=4" title="clovis1122" width="80" height="80"></a>
<a href="https://github.com/juliosaraiva"><img src="https://avatars.githubusercontent.com/u/6595701?v=4" title="juliosaraiva" width="80" height="80"></a>
<a href="https://github.com/Pranay0302"><img src="https://avatars.githubusercontent.com/u/55592629?v=4" title="Pranay0302" width="80" height="80"></a>
<a href="https://github.com/webstradev"><img src="https://avatars.githubusercontent.com/u/82543732?v=4" title="webstradev" width="80" height="80"></a>
<a href="https://github.com/geoah"><img src="https://avatars.githubusercontent.com/u/88447?v=4" title="geoah" width="80" height="80"></a>
<a href="https://github.com/s4kibs4mi"><img src="https://avatars.githubusercontent.com/u/5650785?v=4" title="s4kibs4mi" width="80" height="80"></a>
<a href="https://github.com/SusyQinqinYang"><img src="https://avatars.githubusercontent.com/u/55719616?v=4" title="SusyQinqinYang" width="80" height="80"></a>
<a href="https://github.com/yashrsharma44"><img src="https://avatars.githubusercontent.com/u/31438680?v=4" title="yashrsharma44" width="80" height="80"></a>
<a href="https://github.com/wusphinx"><img src="https://avatars.githubusercontent.com/u/1380777?v=4" title="wusphinx" width="80" height="80"></a>
<a href="https://github.com/ayeniblessing101"><img src="https://avatars.githubusercontent.com/u/29165344?v=4" title="ayeniblessing101" width="80" height="80"></a>
<a href="https://github.com/awwalker"><img src="https://avatars.githubusercontent.com/u/11507633?v=4" title="awwalker" width="80" height="80"></a>
<a href="https://github.com/appleboy"><img src="https://avatars.githubusercontent.com/u/21979?v=4" title="appleboy" width="80" height="80"></a>
<a href="https://github.com/highb"><img src="https://avatars.githubusercontent.com/u/759848?v=4" title="highb" width="80" height="80"></a>
<a href="https://github.com/waywardmonkeys"><img src="https://avatars.githubusercontent.com/u/178582?v=4" title="waywardmonkeys" width="80" height="80"></a>
<a href="https://github.com/cfbolz"><img src="https://avatars.githubusercontent.com/u/85942?v=4" title="cfbolz" width="80" height="80"></a>
<a href="https://github.com/charlesverdad"><img src="https://avatars.githubusercontent.com/u/382186?v=4" title="charlesverdad" width="80" height="80"></a>
<a href="https://github.com/cwalv"><img src="https://avatars.githubusercontent.com/u/887222?v=4" title="cwalv" width="80" height="80"></a>
<a href="https://github.com/Dzalevski"><img src="https://avatars.githubusercontent.com/u/9572827?v=4" title="Dzalevski" width="80" height="80"></a>
<a href="https://github.com/dhanusaputra"><img src="https://avatars.githubusercontent.com/u/35093673?v=4" title="dhanusaputra" width="80" height="80"></a>
<a href="https://github.com/Juneezee"><img src="https://avatars.githubusercontent.com/u/20135478?v=4" title="Juneezee" width="80" height="80"></a>
<a href="https://github.com/Faria-Ejaz"><img src="https://avatars.githubusercontent.com/u/14238844?v=4" title="Faria-Ejaz" width="80" height="80"></a>
<a href="https://github.com/gabrielzezze"><img src="https://avatars.githubusercontent.com/u/38350130?v=4" title="gabrielzezze" width="80" height="80"></a>
<a href="https://github.com/yveshield"><img src="https://avatars.githubusercontent.com/u/8733258?v=4" title="yveshield" width="80" height="80"></a>
<a href="https://github.com/czeslavo"><img src="https://avatars.githubusercontent.com/u/8835851?v=4" title="czeslavo" width="80" height="80"></a>
<a href="https://github.com/hlts2"><img src="https://avatars.githubusercontent.com/u/25459661?v=4" title="hlts2" width="80" height="80"></a>
<a href="https://github.com/johnduhart"><img src="https://avatars.githubusercontent.com/u/113642?v=4" title="johnduhart" width="80" height="80"></a>
<a href="https://github.com/radixdev"><img src="https://avatars.githubusercontent.com/u/2373546?v=4" title="radixdev" width="80" height="80"></a>
<a href="https://github.com/louisphn"><img src="https://avatars.githubusercontent.com/u/72560298?v=4" title="louisphn" width="80" height="80"></a>
<a href="https://github.com/Gookuruto"><img src="https://avatars.githubusercontent.com/u/25951216?v=4" title="Gookuruto" width="80" height="80"></a>
<a href="https://github.com/mhansen"><img src="https://avatars.githubusercontent.com/u/105529?v=4" title="mhansen" width="80" height="80"></a>
<a href="https://github.com/kavu"><img src="https://avatars.githubusercontent.com/u/1994?v=4" title="kavu" width="80" height="80"></a>
<a href="https://github.com/proggga"><img src="https://avatars.githubusercontent.com/u/12262156?v=4" title="proggga" width="80" height="80"></a>
<a href="https://github.com/samoilenko"><img src="https://avatars.githubusercontent.com/u/4024256?v=4" title="samoilenko" width="80" height="80"></a>
<a href="https://github.com/teivah"><img src="https://avatars.githubusercontent.com/u/934784?v=4" title="teivah" width="80" height="80"></a>
<a href="https://github.com/NSObjects"><img src="https://avatars.githubusercontent.com/u/17995427?v=4" title="NSObjects" width="80" height="80"></a>
<a href="https://github.com/Tusharkshahi"><img src="https://avatars.githubusercontent.com/u/103762351?v=4" title="Tusharkshahi" width="80" height="80"></a>
<a href="https://github.com/vbehar"><img src="https://avatars.githubusercontent.com/u/6251?v=4" title="vbehar" width="80" height="80"></a>
<a href="https://github.com/cuishuang"><img src="https://avatars.githubusercontent.com/u/15921519?v=4" title="cuishuang" width="80" height="80"></a>
<a href="https://github.com/apps/dependabot"><img src="https://avatars.githubusercontent.com/in/29110?v=4" title="dependabot[bot]" width="80" height="80"></a>
<a href="https://github.com/futurelm"><img src="https://avatars.githubusercontent.com/u/43361929?v=4" title="futurelm" width="80" height="80"></a>
<a href="https://github.com/hiyanxu"><img src="https://avatars.githubusercontent.com/u/15027927?v=4" title="hiyanxu" width="80" height="80"></a>
<a href="https://github.com/jakemcf22"><img src="https://avatars.githubusercontent.com/u/108971885?v=4" title="jakemcf22" width="80" height="80"></a>
<a href="https://github.com/miravtmehta"><img src="https://avatars.githubusercontent.com/u/54740656?v=4" title="miravtmehta" width="80" height="80"></a>
<a href="https://github.com/lzh2nix"><img src="https://avatars.githubusercontent.com/u/7421004?v=4" title="lzh2nix" width="80" height="80"></a>
<a href="https://github.com/cnych"><img src="https://avatars.githubusercontent.com/u/3094973?v=4" title="cnych" width="80" height="80"></a>

[//]: contributor-faces
