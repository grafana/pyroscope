<p align="center"><img alt="Pyroscope" src="https://user-images.githubusercontent.com/662636/105129037-11334180-5a99-11eb-8951-1d4aaaed50de.png" width="500px"/></p>


[![Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3ATests)
[![Apache 2 License](https://img.shields.io/badge/license-Apache%202-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/release/pyroscope-io/pyroscope.svg)](https://github.com/pyroscope-io/pyroscope/releases)
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
  <a href="https://pyroscope.io/slack">SLACK</a>
</h2>

Pyroscope 是一个开源的持续性能剖析平台。它能够帮你：
* 找出源代码中的性能问题
* 解決 CPU 过度使用的问题
* 理解应用程序的调用树（call tree）
* 追踪随时间变化的情况

## 演示

[🔥 Pyroscope 的演示网站 🔥](https://demo.pyroscope.io/)

[![Pyroscope GIF Demo](https://user-images.githubusercontent.com/662636/105124618-55b9df80-5a8f-11eb-8ad5-0e18c17c827d.gif)](https://demo.pyroscope.io/)


## 特性

* 可以存储下多个应用程序长时间的 profiling 数据
* 你可以一次查看多年的数据或单独查看特定的事件
* 较低的 CPU 开销
* 数据压缩效率高，磁盘空间要求低
* 友好体验的 UI
* 支持 Go、Ruby 和 Python

## 只需3个步骤在本地使用 Pyroscope

```shell
# 安装 pyroscope
brew install pyroscope-io/brew/pyroscope

# 启动 pyroscope server:
pyroscope server

# 在另外一个终端页面，启动 profilling 应用：
pyroscope exec python manage.py runserver
```

## 文档

关于如何在其他编程语言中使用 Pyroscope、在 Linux 上安装它，或在生产环境中使用它的更多信息，请查看我们的文档。

* [文档](https://pyroscope.io/docs/)
* [部署](https://pyroscope.io/docs/deployment)
* [开发人员指导](https://pyroscope.io/docs/developer-guide)


## 部署架构

![agent_server_diagram_00-01-01](https://user-images.githubusercontent.com/23323466/119034061-d536bd00-b962-11eb-8642-ce23a1b6b35f.png)


## 下载

你可以从我们的[下载页面]((https://pyroscope.io/downloads/))下载最新版本的 pyroscope，可以用于 MacOS、Linux 和 Docker 环境使用。


## 兼容性

* [x] Ruby
* [x] Python
* [x] Go
* [x] Linux eBPF
* [ ] Node (即将支持)


你也可以在 [issue](https://github.com/pyroscope-io/pyroscope/issues?q=is%3Aissue+is%3Aopen+label%3Anew-profilers) 或者我们的 [slack](https://pyroscope.io/slack) 中来告诉我们你还想支持的平台。


## 贡献

在为我们贡献代码之前，请先查看我们的[贡献指南](/CONTRIBUTING.md)。


### 感谢 Pyroscope 的贡献者!

[//]: contributor-faces
<a href="https://github.com/petethepig"><img src="https://avatars.githubusercontent.com/u/662636?v=4" title="petethepig" width="80" height="80"></a>
<a href="https://github.com/Rperry2174"><img src="https://avatars.githubusercontent.com/u/23323466?v=4" title="Rperry2174" width="80" height="80"></a>
<a href="https://github.com/LouisInFlow"><img src="https://avatars.githubusercontent.com/u/73438887?v=4" title="LouisInFlow" width="80" height="80"></a>
<a href="https://github.com/abaali"><img src="https://avatars.githubusercontent.com/u/37961057?v=4" title="abaali" width="80" height="80"></a>
<a href="https://github.com/ekpatrice"><img src="https://avatars.githubusercontent.com/u/77462462?v=4" title="ekpatrice" width="80" height="80"></a>
<a href="https://github.com/cmonez"><img src="https://avatars.githubusercontent.com/u/39146411?v=4" title="cmonez" width="80" height="80"></a>
<a href="https://github.com/Pranay0302"><img src="https://avatars.githubusercontent.com/u/55592629?v=4" title="Pranay0302" width="80" height="80"></a>
<a href="https://github.com/geoah"><img src="https://avatars.githubusercontent.com/u/88447?v=4" title="geoah" width="80" height="80"></a>
<a href="https://github.com/yveshield"><img src="https://avatars.githubusercontent.com/u/8733258?v=4" title="yveshield" width="80" height="80"></a>

[//]: contributor-faces
