
<p align="center"><img alt="Pyroscope" src="https://user-images.githubusercontent.com/662636/105129037-11334180-5a99-11eb-8951-1d4aaaed50de.png" width="500px"/></p>


[![Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3ATests)
[![Apache 2 License](https://img.shields.io/badge/license-Apache%202-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/release/pyroscope-io/pyroscope.svg)](https://github.com/pyroscope-io/pyroscope/releases)
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

Pyroscope is an open source continuous profiling platform. It will help you:
* Find performance issues in your code
* Resolve issues with high CPU utilization
* Understand the call tree of your application
* Track changes over time


## Live Demo

<h2>
  🔥<a href="https://demo.pyroscope.io/?name=hotrod.python.frontend%7B%7D">Pyroscope Live Demo</a>🔥
</h2>

[![Pyroscope GIF Demo](https://user-images.githubusercontent.com/662636/105124618-55b9df80-5a8f-11eb-8ad5-0e18c17c827d.gif)](https://demo.pyroscope.io/)


## Features

* Can store years of profiling data from multiple applications
* You can look at years of data at a time or zoom in on specific events
* Low CPU overhead
* Efficient compression, low disk space requirements
* Snappy UI
* Support for Go, Ruby and Python

## Try pyroscope locally in 3 steps:

```shell
# install pyroscope
brew install pyroscope-io/brew/pyroscope

# start pyroscope server:
pyroscope server

# in a separate tab, start profiling your app:
pyroscope exec python manage.py runserver
```

## Documentation

For more information on how to use Pyroscope with other programming languages, install it on Linux or use it in production environment check out our documentation:

* [Getting Started](https://pyroscope.io/docs/)
* [Deployment Guide](https://pyroscope.io/docs/deployment)
* [Developer Guide](https://pyroscope.io/docs/developer-guide)


## Deployment Diagram

![Deployment Diagram](.github/markdown-images/deployment.svg)

## Downloads

You can download the latest version of pyroscope for macOS, linux and Docker from our [Downloads page](https://pyroscope.io/downloads/).

## Supported Integrations

* [x] Ruby
* [x] Python
* [x] Go
* [ ] Node (coming soon)
* [ ] Linux eBPF (coming soon)

Let us know what other integrations you want to see in [our slack](https://pyroscope.io/slack).

## Contributing

To start contributing, check out our [Contributing Guide](/CONTRIBUTING.md)

### Thanks to the contributors of Pyroscope!

[//]: contributor-faces
<a href="https://github.com/petethepig"><img src="https://avatars.githubusercontent.com/u/662636?v=4" title="petethepig" width="80" height="80"></a>
<a href="https://github.com/Rperry2174"><img src="https://avatars.githubusercontent.com/u/23323466?v=4" title="Rperry2174" width="80" height="80"></a>
<a href="https://github.com/LouisInFlow"><img src="https://avatars.githubusercontent.com/u/73438887?v=4" title="LouisInFlow" width="80" height="80"></a>
<a href="https://github.com/abaali"><img src="https://avatars.githubusercontent.com/u/37961057?v=4" title="abaali" width="80" height="80"></a>
<a href="https://github.com/ekpatrice"><img src="https://avatars.githubusercontent.com/u/77462462?v=4" title="ekpatrice" width="80" height="80"></a>
<a href="https://github.com/cmonez"><img src="https://avatars.githubusercontent.com/u/39146411?v=4" title="cmonez" width="80" height="80"></a>
<a href="https://github.com/Pranay0302"><img src="https://avatars.githubusercontent.com/u/55592629?v=4" title="Pranay0302" width="80" height="80"></a>
<a href="https://github.com/geoah"><img src="https://avatars.githubusercontent.com/u/88447?v=4" title="geoah" width="80" height="80"></a>

[//]: contributor-faces
