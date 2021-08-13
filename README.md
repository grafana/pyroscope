
<p align="center"><img alt="Pyroscope" src="https://user-images.githubusercontent.com/662636/105129037-11334180-5a99-11eb-8951-1d4aaaed50de.png" width="500px"/></p>


[![Go Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/Go%20Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3AGo%20Tests)
[![JS Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/JS%20Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3AJS%20Tests)
[![Go Report](https://goreportcard.com/badge/github.com/pyroscope-io/pyroscope)](https://goreportcard.com/report/github.com/pyroscope-io/pyroscope)
[![Apache 2 License](https://img.shields.io/badge/license-Apache%202-blue.svg)](LICENSE)
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


Pyroscope is an open source continuous profiling platform. It will help you:
* Find performance issues in your code
* Resolve issues with high CPU utilization
* Understand the call tree of your application
* Track changes over time


## 🔥 [Pyroscope Live Demo](https://demo.pyroscope.io/?name=hotrod.python.frontend%7B%7D) 🔥

[![Pyroscope GIF Demo](https://user-images.githubusercontent.com/662636/105124618-55b9df80-5a8f-11eb-8ad5-0e18c17c827d.gif)](https://demo.pyroscope.io/)


## Features

* Can store years of profiling data from multiple applications
* You can look at years of data at a time or zoom in on specific events
* Low CPU overhead
* Efficient compression, low disk space requirements
* Snappy UI
* Support for Go, Ruby and Python

## Try Pyroscope locally in 3 steps:

```shell
# install pyroscope
brew install pyroscope-io/brew/pyroscope

# start pyroscope server:
pyroscope server

# in a separate tab, start profiling your app:
pyroscope exec python manage.py runserver # If using Python
pyroscope exec rails server               # If using Ruby

# If using Pyroscope cloud add flags for server address and auth token
# pyroscope exec -server-address "https://your_company.pyroscope.cloud" -auth-token "ps-key-1234567890" python manage.py runserver
```

## Documentation

For more information on how to use Pyroscope with other programming languages, install it on Linux, or use it in production environment, check out our documentation:

* [Public Roadmap](https://github.com/pyroscope-io/pyroscope/projects/1)
* [Getting Started](https://pyroscope.io/docs/)
* [Deployment Guide](https://pyroscope.io/docs/deployment)
* [Developer Guide](https://pyroscope.io/docs/developer-guide)


## Deployment Diagram

![Deployment Diagram](.github/markdown-images/deployment.svg)

## Downloads

You can download the latest version of pyroscope for macOS, linux and Docker from our [Downloads page](https://pyroscope.io/downloads/).

## Supported Integrations

* [x] Ruby (via `rbspy`)
* [x] Python (via `py-spy`)
* [x] Go (via `pprof`)
* [x] Linux eBPF (via `profile.py` from `bcc-tools`)
* [x] PHP (via `phpspy`)
* [x] .NET (via `dotnet trace`)
* [ ] Java (coming soon)

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
<a href="https://github.com/Rperry2174"><img src="https://avatars.githubusercontent.com/u/23323466?v=4" title="Rperry2174" width="80" height="80"></a>
<a href="https://github.com/kolesnikovae"><img src="https://avatars.githubusercontent.com/u/12090599?v=4" title="kolesnikovae" width="80" height="80"></a>
<a href="https://github.com/LouisInFlow"><img src="https://avatars.githubusercontent.com/u/84481279?v=4" title="LouisInFlow" width="80" height="80"></a>
<a href="https://github.com/abaali"><img src="https://avatars.githubusercontent.com/u/37961057?v=4" title="abaali" width="80" height="80"></a>
<a href="https://github.com/olvrng"><img src="https://avatars.githubusercontent.com/u/6618620?v=4" title="olvrng" width="80" height="80"></a>
<a href="https://github.com/alonlong"><img src="https://avatars.githubusercontent.com/u/3090383?v=4" title="alonlong" width="80" height="80"></a>
<a href="https://github.com/AdrK"><img src="https://avatars.githubusercontent.com/u/15175440?v=4" title="AdrK" width="80" height="80"></a>
<a href="https://github.com/cjsampson"><img src="https://avatars.githubusercontent.com/u/8391857?v=4" title="cjsampson" width="80" height="80"></a>
<a href="https://github.com/Loggy"><img src="https://avatars.githubusercontent.com/u/3171097?v=4" title="Loggy" width="80" height="80"></a>
<a href="https://github.com/ekpatrice"><img src="https://avatars.githubusercontent.com/u/77462462?v=4" title="ekpatrice" width="80" height="80"></a>
<a href="https://github.com/cmonez"><img src="https://avatars.githubusercontent.com/u/39146411?v=4" title="cmonez" width="80" height="80"></a>
<a href="https://github.com/rajat2004"><img src="https://avatars.githubusercontent.com/u/37938604?v=4" title="rajat2004" width="80" height="80"></a>
<a href="https://github.com/Pranay0302"><img src="https://avatars.githubusercontent.com/u/55592629?v=4" title="Pranay0302" width="80" height="80"></a>
<a href="https://github.com/geoah"><img src="https://avatars.githubusercontent.com/u/88447?v=4" title="geoah" width="80" height="80"></a>
<a href="https://github.com/s4kibs4mi"><img src="https://avatars.githubusercontent.com/u/5650785?v=4" title="s4kibs4mi" width="80" height="80"></a>
<a href="https://github.com/SusyQinqinYang"><img src="https://avatars.githubusercontent.com/u/55719616?v=4" title="SusyQinqinYang" width="80" height="80"></a>
<a href="https://github.com/eh-am"><img src="https://avatars.githubusercontent.com/u/6951209?v=4" title="eh-am" width="80" height="80"></a>
<a href="https://github.com/wusphinx"><img src="https://avatars.githubusercontent.com/u/1380777?v=4" title="wusphinx" width="80" height="80"></a>
<a href="https://github.com/Skemba"><img src="https://avatars.githubusercontent.com/u/8813875?v=4" title="Skemba" width="80" height="80"></a>
<a href="https://github.com/ayeniblessing101"><img src="https://avatars.githubusercontent.com/u/29165344?v=4" title="ayeniblessing101" width="80" height="80"></a>
<a href="https://github.com/appleboy"><img src="https://avatars.githubusercontent.com/u/21979?v=4" title="appleboy" width="80" height="80"></a>
<a href="https://github.com/highb"><img src="https://avatars.githubusercontent.com/u/759848?v=4" title="highb" width="80" height="80"></a>
<a href="https://github.com/cwalv"><img src="https://avatars.githubusercontent.com/u/887222?v=4" title="cwalv" width="80" height="80"></a>
<a href="https://github.com/Faria-Ejaz"><img src="https://avatars.githubusercontent.com/u/14238844?v=4" title="Faria-Ejaz" width="80" height="80"></a>
<a href="https://github.com/yveshield"><img src="https://avatars.githubusercontent.com/u/8733258?v=4" title="yveshield" width="80" height="80"></a>
<a href="https://github.com/czeslavo"><img src="https://avatars.githubusercontent.com/u/8835851?v=4" title="czeslavo" width="80" height="80"></a>
<a href="https://github.com/johnduhart"><img src="https://avatars.githubusercontent.com/u/113642?v=4" title="johnduhart" width="80" height="80"></a>
<a href="https://github.com/radixdev"><img src="https://avatars.githubusercontent.com/u/2373546?v=4" title="radixdev" width="80" height="80"></a>
<a href="https://github.com/NSObjects"><img src="https://avatars.githubusercontent.com/u/17995427?v=4" title="NSObjects" width="80" height="80"></a>
<a href="https://github.com/vbehar"><img src="https://avatars.githubusercontent.com/u/6251?v=4" title="vbehar" width="80" height="80"></a>
<a href="https://github.com/yashrsharma44"><img src="https://avatars.githubusercontent.com/u/31438680?v=4" title="yashrsharma44" width="80" height="80"></a>
<a href="https://github.com/hiyanxu"><img src="https://avatars.githubusercontent.com/u/15027927?v=4" title="hiyanxu" width="80" height="80"></a>
<a href="https://github.com/miravtmehta"><img src="https://avatars.githubusercontent.com/u/54740656?v=4" title="miravtmehta" width="80" height="80"></a>
<a href="https://github.com/lzh2nix"><img src="https://avatars.githubusercontent.com/u/7421004?v=4" title="lzh2nix" width="80" height="80"></a>
<a href="https://github.com/cnych"><img src="https://avatars.githubusercontent.com/u/3094973?v=4" title="cnych" width="80" height="80"></a>

[//]: contributor-faces
