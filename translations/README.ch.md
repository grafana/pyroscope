<p align="center"><img alt="Pyroscope" src="https://user-images.githubusercontent.com/662636/105129037-11334180-5a99-11eb-8951-1d4aaaed50de.png" width="500px"/></p>


[![Tests Status](https://github.com/pyroscope-io/pyroscope/workflows/Tests/badge.svg)](https://github.com/pyroscope-io/pyroscope/actions?query=workflow%3ATests)
[![Apache 2 License](https://img.shields.io/badge/license-Apache%202-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/release/pyroscope-io/pyroscope.svg)](https://github.com/pyroscope-io/pyroscope/releases)
[![GoDoc](https://godoc.org/github.com/pyroscope-io/pyroscope?status.svg)](https://godoc.org/github.com/pyroscope-io/pyroscope)

<h2>
  <a href="https://pyroscope.io/">網路</a>
  <span> • </span>
  <a href="https://pyroscope.io/docs">芝料</a>
  <span> • </span>
  <a href="https://demo.pyroscope.io/">示範</a>
  <span> • </span>
  <a href="/examples">範例</a>
  <span> • </span>
  <a href="https://pyroscope.io/slack">SLACK</a>
</h2>

Pyroscope 是個開放源碼的連續側寫網路平台。它能夠幫你：
* 挑出源碼的性能錯誤
* 解決CPU過度利用的問題
* 理解應用程式的call tree
* 一直跟蹤軟體裡的變化

## 演示芝料

[🔥 Pyroscope的演示芝料 🔥](https://demo.pyroscope.io/)

[![Pyroscope GIF Demo](https://user-images.githubusercontent.com/662636/105124618-55b9df80-5a8f-11eb-8ad5-0e18c17c827d.gif)](https://demo.pyroscope.io/)


## 機能

* 能夠存下好幾年來多個應用程式累積出來的資料
* 能夠讓你一次看見好幾年來的資料或著單單看個別的事件
* CPU使用數量低
* 數據壓縮效率高，軟盤的空間需求低
* 光滑的UI
* Go、Ruby、Python都兼容

## 在自己電腦上試Pyroscope的三步驟：

```shell
# install pyroscope
brew install pyroscope-io/brew/pyroscope

# start pyroscope server:
pyroscope server

# in a separate tab, start profiling your app:
pyroscope exec python manage.py runserver
```

## 說明書

如果想找Pyroscope跟其他程式語言的用法、灌到Linux上、或著在生產環境裡用法的說明，請查看我們的說明書：
* [起點](https://pyroscope.io/docs/)
* [部署說明書](https://pyroscope.io/docs/deployment)
* [開發人員說明書](https://pyroscope.io/docs/developer-guide)


## 部署圖樣

![Deployment Diagram](.github/markdown-images/deployment.svg)

## 怎樣下載

發展給MacOS、Linux、和Docker最新版的Pyroscope在下載頁面上能下載 [Downloads page](https://pyroscope.io/downloads/).

## 兼容的程式語言

* [x] Ruby
* [x] Python
* [x] Go
* [ ] Node (即將到來)
* [ ] Linux eBPF (即將到來)

請在我們的Slack上告訴我們你還想看倒哪些程式語言 [our slack](https://pyroscope.io/slack).

## 怎樣貢獻

如果想當貢獻者，請查看我們的貢獻說明書。 [Contributing Guide](/CONTRIBUTING.md)

### 感謝全部幫助發展Pyroscope的貢獻者！

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
