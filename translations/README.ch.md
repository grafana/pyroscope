
<p align="center"><img alt="Pyroscope" src="https://user-images.githubusercontent.com/662636/105129037-11334180-5a99-11eb-8951-1d4aaaed50de.png" width="500px"/></p>

[![Go Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/Go%20Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3AGo%20Tests)
[![JS Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/JS%20Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3AJS%20Tests)
[![Go Report](https://goreportcard.com/badge/github.com/pyroscope-io/pyroscope)](https://goreportcard.com/report/github.com/pyroscope-io/pyroscope)
[![Apache 2 License](https://img.shields.io/badge/license-Apache%202-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/release/pyroscope-io/pyroscope.svg)](https://github.com/pyroscope-io/pyroscope/releases)
[![DockerHub](https://img.shields.io/docker/pulls/pyroscope/pyroscope.svg)](https://hub.docker.com/r/pyroscope/pyroscope)
[![GoDoc](https://godoc.org/github.com/pyroscope-io/pyroscope?status.svg)](https://godoc.org/github.com/pyroscope-io/pyroscope)

<h2>
  <a href="https://pyroscope.io/">官网</a>
  <span> • </span>
  <a href="https://pyroscope.io/docs">文档</a>
  <span> • </span>
  <a href="https://demo.pyroscope.io/">演示</a>
  <span> • </span>
  <a href="/examples">示例</a>
  <span> • </span>
  <a href="https://pyroscope.io/slack">Slack</a>
</h2>


### 什么是 Pyroscope?
Pyroscope 是一个开源的持续性能剖析平台。它能够帮你：
* 找出源代码中的性能问题和瓶颈
* 解决 CPU 利用率高的问题
* 理解应用程序的调用树（call tree）
* 追踪随一段时间内变化的情况

## 🔥 [Pyroscope 在线演示](https://demo.pyroscope.io/?name=hotrod.python.frontend%7B%7D) 🔥

[![Pyroscope GIF Demo](https://user-images.githubusercontent.com/662636/105124618-55b9df80-5a8f-11eb-8ad5-0e18c17c827d.gif)](https://demo.pyroscope.io/)


## 特性

* 可以存储来自多个应用程序的多年剖析数据
* 你可以一次查看多年的数据或单独查看特定的事件
* 较低的 CPU 开销
* 数据压缩效率高，磁盘空间要求低
* 快捷的 UI 界面

## 通过2个步骤在本地添加 Pyroscope Server:
Pyroscope 支持所有主要的计算机架构，并且非常容易安装。作为例子，以下是在 Mac 上的安装方法:
```shell
# 安装 pyroscope
brew install pyroscope-io/brew/pyroscope

# 启动 pyroscope server:
pyroscope server
```

## 通过 Pyroscope agent 发送数据到 server（特定语言）
关于如何将 Pyroscope agent 添加到你的代码中的更多信息，请参见我们网站上的[agent 文档](https://pyroscope.io/docs/agent-overview) 。
- [Golang Agent](https://pyroscope.io/docs/golang)
- [Python Agent (pip)](https://pyroscope.io/docs/python)
- [Ruby Agent (gem)](https://pyroscope.io/docs/ruby)
- [eBPF Agent](https://pyroscope.io/docs/ebpf)
- [PHP Agent](https://pyroscope.io/docs/php)
- [.NET Agent](https://pyroscope.io/docs/dotnet)

## 文档

关于如何在其他编程语言中使用 Pyroscope, 在 Linux 上安装它，或在生产环境中使用它的更多信息，请查看我们的文档。

* [公开的 Roadmap](https://github.com/pyroscope-io/pyroscope/projects/1)
* [入门文档](https://pyroscope.io/docs/)
* [部署指导](https://pyroscope.io/docs/deployment)
* [开发人员指导](https://pyroscope.io/docs/developer-guide)


## 部署示意图

![agent_server_diagram_10](https://user-images.githubusercontent.com/23323466/153685751-0aac3cd6-bbc1-4ab4-8350-8f4dc7f7c193.svg)

## 下载

你可以从我们的 [下载页面](https://pyroscope.io/downloads/) 下载适用于macOS、linux和Docker的最新版本的 pyroscope。

## 已支持的集成

* [x] Ruby (通过 `rbspy`)
* [x] Python (通过 `py-spy`)
* [x] Go (通过 `pprof`)
* [x] Linux eBPF (通过`bcc-tools`的`profile.py`)
* [x] PHP (通过 `phpspy`)
* [x] .NET (通过 `dotnet trace`)
* [x] Java (通过 `async-profiler`)
* [ ] Node [(寻找贡献者)](https://github.com/pyroscope-io/pyroscope/issues/8)


你也可以在 [issue](https://github.com/pyroscope-io/pyroscope/issues?q=is%3Aissue+is%3Aopen+label%3Anew-profilers) 或者我们的 [slack](https://pyroscope.io/slack) 中来告诉我们你还想支持的平台。

## 鸣谢

Pyroscope 的出现要感谢许多人的出色工作，包括但不限于：

* Brendan Gregg - Flame Graphs 的发明者
* Julia Evans - rbspy 的创造者 - Ruby 的采样分析器
* Vladimir Agafonkin --flamebearer的创造者 --快速火焰图的渲染器
* Ben Frederickson - py-spy 的创造者 - Python 的采样分析器
* Adam Saponara - phpspy 的创造者 - PHP 的抽样分析器
* Alexei Starovoitov, Brendan Gregg, 和其他许多人，他们使 Linux 内核中基于 BPF 的剖析成为可能。


## 贡献

在为我们贡献代码之前，请先查看我们的[贡献指南](../CONTRIBUTING.md)。


### 感谢 Pyroscope 的贡献者!

[//]: contributor-faces
<a href="https://github.com/petethepig"><img src="https://avatars.githubusercontent.com/u/662636?v=4" title="petethepig" width="80" height="80"></a>
<a href="https://github.com/Rperry2174"><img src="https://avatars.githubusercontent.com/u/23323466?v=4" title="Rperry2174" width="80" height="80"></a>
<a href="https://github.com/kolesnikovae"><img src="https://avatars.githubusercontent.com/u/12090599?v=4" title="kolesnikovae" width="80" height="80"></a>
<a href="https://github.com/eh-am"><img src="https://avatars.githubusercontent.com/u/6951209?v=4" title="eh-am" width="80" height="80"></a>
<a href="https://github.com/LouisInFlow"><img src="https://avatars.githubusercontent.com/u/84481279?v=4" title="LouisInFlow" width="80" height="80"></a>
<a href="https://github.com/abaali"><img src="https://avatars.githubusercontent.com/u/37961057?v=4" title="abaali" width="80" height="80"></a>
<a href="https://github.com/olvrng"><img src="https://avatars.githubusercontent.com/u/6618620?v=4" title="olvrng" width="80" height="80"></a>
<a href="https://github.com/alonlong"><img src="https://avatars.githubusercontent.com/u/3090383?v=4" title="alonlong" width="80" height="80"></a>
<a href="https://github.com/Loggy"><img src="https://avatars.githubusercontent.com/u/3171097?v=4" title="Loggy" width="80" height="80"></a>
<a href="https://github.com/AdrK"><img src="https://avatars.githubusercontent.com/u/15175440?v=4" title="AdrK" width="80" height="80"></a>
<a href="https://github.com/cjsampson"><img src="https://avatars.githubusercontent.com/u/8391857?v=4" title="cjsampson" width="80" height="80"></a>
<a href="https://github.com/ekpatrice"><img src="https://avatars.githubusercontent.com/u/77462462?v=4" title="ekpatrice" width="80" height="80"></a>
<a href="https://github.com/cmonez"><img src="https://avatars.githubusercontent.com/u/39146411?v=4" title="cmonez" width="80" height="80"></a>
<a href="https://github.com/rajat2004"><img src="https://avatars.githubusercontent.com/u/37938604?v=4" title="rajat2004" width="80" height="80"></a>
<a href="https://github.com/Pranay0302"><img src="https://avatars.githubusercontent.com/u/55592629?v=4" title="Pranay0302" width="80" height="80"></a>
<a href="https://github.com/ruslanpascoal2"><img src="https://avatars.githubusercontent.com/u/61955096?v=4" title="ruslanpascoal2" width="80" height="80"></a>
<a href="https://github.com/Skemba"><img src="https://avatars.githubusercontent.com/u/8813875?v=4" title="Skemba" width="80" height="80"></a>
<a href="https://github.com/geoah"><img src="https://avatars.githubusercontent.com/u/88447?v=4" title="geoah" width="80" height="80"></a>
<a href="https://github.com/s4kibs4mi"><img src="https://avatars.githubusercontent.com/u/5650785?v=4" title="s4kibs4mi" width="80" height="80"></a>
<a href="https://github.com/SusyQinqinYang"><img src="https://avatars.githubusercontent.com/u/55719616?v=4" title="SusyQinqinYang" width="80" height="80"></a>
<a href="https://github.com/yashrsharma44"><img src="https://avatars.githubusercontent.com/u/31438680?v=4" title="yashrsharma44" width="80" height="80"></a>
<a href="https://github.com/wusphinx"><img src="https://avatars.githubusercontent.com/u/1380777?v=4" title="wusphinx" width="80" height="80"></a>
<a href="https://github.com/ayeniblessing101"><img src="https://avatars.githubusercontent.com/u/29165344?v=4" title="ayeniblessing101" width="80" height="80"></a>
<a href="https://github.com/appleboy"><img src="https://avatars.githubusercontent.com/u/21979?v=4" title="appleboy" width="80" height="80"></a>
<a href="https://github.com/highb"><img src="https://avatars.githubusercontent.com/u/759848?v=4" title="highb" width="80" height="80"></a>
<a href="https://github.com/cwalv"><img src="https://avatars.githubusercontent.com/u/887222?v=4" title="cwalv" width="80" height="80"></a>
<a href="https://github.com/Faria-Ejaz"><img src="https://avatars.githubusercontent.com/u/14238844?v=4" title="Faria-Ejaz" width="80" height="80"></a>
<a href="https://github.com/yveshield"><img src="https://avatars.githubusercontent.com/u/8733258?v=4" title="yveshield" width="80" height="80"></a>
<a href="https://github.com/czeslavo"><img src="https://avatars.githubusercontent.com/u/8835851?v=4" title="czeslavo" width="80" height="80"></a>
<a href="https://github.com/johnduhart"><img src="https://avatars.githubusercontent.com/u/113642?v=4" title="johnduhart" width="80" height="80"></a>
<a href="https://github.com/radixdev"><img src="https://avatars.githubusercontent.com/u/2373546?v=4" title="radixdev" width="80" height="80"></a>
<a href="https://github.com/teivah"><img src="https://avatars.githubusercontent.com/u/934784?v=4" title="teivah" width="80" height="80"></a>
<a href="https://github.com/NSObjects"><img src="https://avatars.githubusercontent.com/u/17995427?v=4" title="NSObjects" width="80" height="80"></a>
<a href="https://github.com/vbehar"><img src="https://avatars.githubusercontent.com/u/6251?v=4" title="vbehar" width="80" height="80"></a>
<a href="https://github.com/gawicks"><img src="https://avatars.githubusercontent.com/u/1481491?v=4" title="gawicks" width="80" height="80"></a>
<a href="https://github.com/hiyanxu"><img src="https://avatars.githubusercontent.com/u/15027927?v=4" title="hiyanxu" width="80" height="80"></a>
<a href="https://github.com/miravtmehta"><img src="https://avatars.githubusercontent.com/u/54740656?v=4" title="miravtmehta" width="80" height="80"></a>
<a href="https://github.com/lzh2nix"><img src="https://avatars.githubusercontent.com/u/7421004?v=4" title="lzh2nix" width="80" height="80"></a>
<a href="https://github.com/cnych"><img src="https://avatars.githubusercontent.com/u/3094973?v=4" title="cnych" width="80" height="80"></a>

[//]: contributor-faces
