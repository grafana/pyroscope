## <small>0.0.37 (2021-08-04)</small>

* adds error message ([7e02b3d](https://github.com/pyroscope-io/pyroscope/commit/7e02b3d))
* Fix LFU cache package version ([f2a06a8](https://github.com/pyroscope-io/pyroscope/commit/f2a06a8))
* improves build stability on linux ([31e4065](https://github.com/pyroscope-io/pyroscope/commit/31e4065))



## <small>0.0.36 (2021-08-03)</small>

* Add a short path for case when not all required labels are present ([7c8e7b0](https://github.com/pyroscope-io/pyroscope/commit/7c8e7b0))
* Add dimension lookup by key labels ([ebdc846](https://github.com/pyroscope-io/pyroscope/commit/ebdc846))
* Add explicit yaml key name for metric export rules ([c02a21d](https://github.com/pyroscope-io/pyroscope/commit/c02a21d))
* Add metric exported scratches ([fa20295](https://github.com/pyroscope-io/pyroscope/commit/fa20295))
* Add option to filter prometheus labels ([ee29c0b](https://github.com/pyroscope-io/pyroscope/commit/ee29c0b))
* add prependBytes and test ([e1f5e3d](https://github.com/pyroscope-io/pyroscope/commit/e1f5e3d))
* add test for storage/tree (#276) ([bde2a99](https://github.com/pyroscope-io/pyroscope/commit/bde2a99)), closes [#276](https://github.com/pyroscope-io/pyroscope/issues/276)
* Add tree node filter to metric exporter. ([f76f9c1](https://github.com/pyroscope-io/pyroscope/commit/f76f9c1))
* adds comments to flamebearer ([fada901](https://github.com/pyroscope-io/pyroscope/commit/fada901))
* Adds markdown linting (dead urls) (#270) ([8c4e0c2](https://github.com/pyroscope-io/pyroscope/commit/8c4e0c2)), closes [#270](https://github.com/pyroscope-io/pyroscope/issues/270)
* adds support for .env files (helpful for debugging things like OAuth integration) ([266d360](https://github.com/pyroscope-io/pyroscope/commit/266d360))
* better slice allocating ([81d10a6](https://github.com/pyroscope-io/pyroscope/commit/81d10a6))
* changes contributors limit in README ([3d159b6](https://github.com/pyroscope-io/pyroscope/commit/3d159b6))
* changes eviction timeout value to 20s and brings runtime.GC back ([3b34e74](https://github.com/pyroscope-io/pyroscope/commit/3b34e74))
* changes flamebearer tooltip to position fixed ([b1ba728](https://github.com/pyroscope-io/pyroscope/commit/b1ba728))
* Clean comments ([9138314](https://github.com/pyroscope-io/pyroscope/commit/9138314))
* decode-resp: decode flamebearer response for debugging (#284) ([8c45c81](https://github.com/pyroscope-io/pyroscope/commit/8c45c81)), closes [#284](https://github.com/pyroscope-io/pyroscope/issues/284)
* Defer buffer put ([077e260](https://github.com/pyroscope-io/pyroscope/commit/077e260))
* downgrades to golang:1.16.4-alpine3.12 ([7a47a33](https://github.com/pyroscope-io/pyroscope/commit/7a47a33))
* Fix dictionary keys ([746871d](https://github.com/pyroscope-io/pyroscope/commit/746871d))
* fixes benchmark script ([d5a3c0e](https://github.com/pyroscope-io/pyroscope/commit/d5a3c0e))
* follow up to #283 ([e0f799a](https://github.com/pyroscope-io/pyroscope/commit/e0f799a)), closes [#283](https://github.com/pyroscope-io/pyroscope/issues/283)
* Hide sensitive data from /config endpoint response ([9dfdd25](https://github.com/pyroscope-io/pyroscope/commit/9dfdd25))
* improve prependTreeNode ([d6d904e](https://github.com/pyroscope-io/pyroscope/commit/d6d904e))
* improve tooltip rendering (#266) ([53ce5f8](https://github.com/pyroscope-io/pyroscope/commit/53ce5f8)), closes [#266](https://github.com/pyroscope-io/pyroscope/issues/266)
* improve tree.Insert ([cc0e5c5](https://github.com/pyroscope-io/pyroscope/commit/cc0e5c5))
* initial version of traffic duplicator ([03f32ff](https://github.com/pyroscope-io/pyroscope/commit/03f32ff))
* Introduce ingester abstraction ([75b27be](https://github.com/pyroscope-io/pyroscope/commit/75b27be))
* Make observer respect sample rate ([8411db8](https://github.com/pyroscope-io/pyroscope/commit/8411db8))
* Make ParseKey validate user input ([fa6048b](https://github.com/pyroscope-io/pyroscope/commit/fa6048b))
* Oauth flow for Google, GitHub and GitLab (#272) ([66ea269](https://github.com/pyroscope-io/pyroscope/commit/66ea269)), closes [#272](https://github.com/pyroscope-io/pyroscope/issues/272)
* Protect /config and /build endpoints, if applicable ([3830233](https://github.com/pyroscope-io/pyroscope/commit/3830233))
* Refactor analytics package to decouple it from controller ([c935964](https://github.com/pyroscope-io/pyroscope/commit/c935964))
* Refactor pprof profile traversal ([33db173](https://github.com/pyroscope-io/pyroscope/commit/33db173))
* Replaced no longer maintained jwt-go with golang-jwt/jwt which is community maintained ([438b07f](https://github.com/pyroscope-io/pyroscope/commit/438b07f))
* Resolve merge conflicts ([b9f4c5e](https://github.com/pyroscope-io/pyroscope/commit/b9f4c5e))
* Setup metric exporter initialization ([6daeafc](https://github.com/pyroscope-io/pyroscope/commit/6daeafc))
* Tags support (#280) ([a41b2b7](https://github.com/pyroscope-io/pyroscope/commit/a41b2b7)), closes [#280](https://github.com/pyroscope-io/pyroscope/issues/280)
* Tidy go mod ([212241b](https://github.com/pyroscope-io/pyroscope/commit/212241b))
* update go version in linux builds ([e44f8c8](https://github.com/pyroscope-io/pyroscope/commit/e44f8c8))
* Update upstream.go ([dd1daa5](https://github.com/pyroscope-io/pyroscope/commit/dd1daa5))
* updates alpine version ([77397d8](https://github.com/pyroscope-io/pyroscope/commit/77397d8))
* updates windows golang version ([94d5c43](https://github.com/pyroscope-io/pyroscope/commit/94d5c43))
* Use byte buffer pool for serialization ([725cfdd](https://github.com/pyroscope-io/pyroscope/commit/725cfdd))
* fix(agent): use the ProfileTypes from the configuration ([88aaed0](https://github.com/pyroscope-io/pyroscope/commit/88aaed0))
* fix(gospy.go): Snapshot add custom_pprof.StopCPUProfile() (#283) ([3a13771](https://github.com/pyroscope-io/pyroscope/commit/3a13771)), closes [#283](https://github.com/pyroscope-io/pyroscope/issues/283)



## <small>0.0.35 (2021-06-29)</small>

* Adds new build and config endpoints (#259) ([aa8c868](https://github.com/pyroscope-io/pyroscope/commit/aa8c868)), closes [#259](https://github.com/pyroscope-io/pyroscope/issues/259)
* Fix signal sending on Windows (#261) ([d94dc82](https://github.com/pyroscope-io/pyroscope/commit/d94dc82)), closes [#261](https://github.com/pyroscope-io/pyroscope/issues/261)
* freezes php docker version ([2af42b3](https://github.com/pyroscope-io/pyroscope/commit/2af42b3))
* lints some code ([cc9a447](https://github.com/pyroscope-io/pyroscope/commit/cc9a447))
* sed linux fix ([d2f669f](https://github.com/pyroscope-io/pyroscope/commit/d2f669f))
* switches to httptest everywhere, hopefully will fix flaky http tests (#267) ([3cc251f](https://github.com/pyroscope-io/pyroscope/commit/3cc251f)), closes [#267](https://github.com/pyroscope-io/pyroscope/issues/267)
* updates contributor / changelog scripts ([bd816a1](https://github.com/pyroscope-io/pyroscope/commit/bd816a1))
* Use own fork of read-process-memory (#246) ([5958327](https://github.com/pyroscope-io/pyroscope/commit/5958327)), closes [#246](https://github.com/pyroscope-io/pyroscope/issues/246)



## <small>0.0.34 (2021-06-23)</small>

* adds linelint (#254) ([a85f6a8](https://github.com/pyroscope-io/pyroscope/commit/a85f6a8)), closes [#254](https://github.com/pyroscope-io/pyroscope/issues/254)
* Document dotnet magic denominator (#255) ([07e72b3](https://github.com/pyroscope-io/pyroscope/commit/07e72b3)), closes [#255](https://github.com/pyroscope-io/pyroscope/issues/255)
* Examples should run spies as non-root users #248 (#251) ([0df4c1d](https://github.com/pyroscope-io/pyroscope/commit/0df4c1d)), closes [#248](https://github.com/pyroscope-io/pyroscope/issues/248) [#251](https://github.com/pyroscope-io/pyroscope/issues/251)
* Fixed wrong time calc in dotnetspy. (#252) ([f1dff48](https://github.com/pyroscope-io/pyroscope/commit/f1dff48)), closes [#252](https://github.com/pyroscope-io/pyroscope/issues/252)
* fixes max memory available detection on linux (#256) ([5bee853](https://github.com/pyroscope-io/pyroscope/commit/5bee853)), closes [#256](https://github.com/pyroscope-io/pyroscope/issues/256)
* Graceful shutdown (#243) ([2723d11](https://github.com/pyroscope-io/pyroscope/commit/2723d11)), closes [#243](https://github.com/pyroscope-io/pyroscope/issues/243)
* Ignore tags at dictionary lookup (#245) ([e756a20](https://github.com/pyroscope-io/pyroscope/commit/e756a20)), closes [#245](https://github.com/pyroscope-io/pyroscope/issues/245)
* Improves generate-sample-config script (#236) ([b78084d](https://github.com/pyroscope-io/pyroscope/commit/b78084d)), closes [#236](https://github.com/pyroscope-io/pyroscope/issues/236)



## <small>0.0.33 (2021-06-17)</small>

* Add phpspy coloring #201 (#209) ([a69794d](https://github.com/pyroscope-io/pyroscope/commit/a69794d)), closes [#201](https://github.com/pyroscope-io/pyroscope/issues/201) [#209](https://github.com/pyroscope-io/pyroscope/issues/209)
* Add README for dotnet examples (#233) ([261f882](https://github.com/pyroscope-io/pyroscope/commit/261f882)), closes [#233](https://github.com/pyroscope-io/pyroscope/issues/233)
* Adds a benchmark suite (#231) ([c7cab40](https://github.com/pyroscope-io/pyroscope/commit/c7cab40)), closes [#231](https://github.com/pyroscope-io/pyroscope/issues/231)
* adds better handling for errors when starting the server, improves logger initialization, optimizes  ([970e706](https://github.com/pyroscope-io/pyroscope/commit/970e706)), closes [#215](https://github.com/pyroscope-io/pyroscope/issues/215)
* adds dotnet to the list of supported integrations ([73a7aad](https://github.com/pyroscope-io/pyroscope/commit/73a7aad))
* Bumps rbspy version to v0.6.0 (#237) ([82ce174](https://github.com/pyroscope-io/pyroscope/commit/82ce174)), closes [#237](https://github.com/pyroscope-io/pyroscope/issues/237)
* Closes #168 - Implement data retention in storage layer (WIP) (#182) ([af9a745](https://github.com/pyroscope-io/pyroscope/commit/af9a745)), closes [#168](https://github.com/pyroscope-io/pyroscope/issues/168) [#182](https://github.com/pyroscope-io/pyroscope/issues/182)
* Disable inlining for golang example (#211) ([80a334a](https://github.com/pyroscope-io/pyroscope/commit/80a334a)), closes [#211](https://github.com/pyroscope-io/pyroscope/issues/211)
* Dynamically resizes cache depending on the amount of RAM available (issue #167) (#213) ([764de1d](https://github.com/pyroscope-io/pyroscope/commit/764de1d)), closes [#167](https://github.com/pyroscope-io/pyroscope/issues/167) [#213](https://github.com/pyroscope-io/pyroscope/issues/213)
* fix #226: support the readiness and liveness for kubernetes (#238) ([7331758](https://github.com/pyroscope-io/pyroscope/commit/7331758)), closes [#226](https://github.com/pyroscope-io/pyroscope/issues/226) [#238](https://github.com/pyroscope-io/pyroscope/issues/238)
* Fix issue #154: Add ability to change sampling rate (#190) ([2c4acf6](https://github.com/pyroscope-io/pyroscope/commit/2c4acf6)), closes [#154](https://github.com/pyroscope-io/pyroscope/issues/154) [#190](https://github.com/pyroscope-io/pyroscope/issues/190)
* fix the issue #188: add the file name and code line (#214) ([63e622f](https://github.com/pyroscope-io/pyroscope/commit/63e622f)), closes [#188](https://github.com/pyroscope-io/pyroscope/issues/188) [#214](https://github.com/pyroscope-io/pyroscope/issues/214) [#188](https://github.com/pyroscope-io/pyroscope/issues/188)
* fixes the issue with missing godepgraph (#227) ([3c519ff](https://github.com/pyroscope-io/pyroscope/commit/3c519ff)), closes [#227](https://github.com/pyroscope-io/pyroscope/issues/227)
* Hotfix/segment cache panic (#222) ([81ba6c4](https://github.com/pyroscope-io/pyroscope/commit/81ba6c4)), closes [#222](https://github.com/pyroscope-io/pyroscope/issues/222)
* Implements ability to change sample rate in go profiler ([f98350d](https://github.com/pyroscope-io/pyroscope/commit/f98350d))
* Make phpspy use direct read from mem (#239) ([e8201ca](https://github.com/pyroscope-io/pyroscope/commit/e8201ca)), closes [#239](https://github.com/pyroscope-io/pyroscope/issues/239)
* Part of: Lint Go Code #49 (#223) ([880e06a](https://github.com/pyroscope-io/pyroscope/commit/880e06a)), closes [#49](https://github.com/pyroscope-io/pyroscope/issues/49) [#223](https://github.com/pyroscope-io/pyroscope/issues/223)
* removes extra logging statements ([5e56f27](https://github.com/pyroscope-io/pyroscope/commit/5e56f27))
* removes logrus dependency from pkg/agent/profiler ([36a7eb4](https://github.com/pyroscope-io/pyroscope/commit/36a7eb4))
* support the compile on linux system, like ubuntu and centos (#202) ([5098f57](https://github.com/pyroscope-io/pyroscope/commit/5098f57)), closes [#202](https://github.com/pyroscope-io/pyroscope/issues/202)
* Update agent server diagram in examples ([9c89c4b](https://github.com/pyroscope-io/pyroscope/commit/9c89c4b))
* Update deployment.svg ([114eabd](https://github.com/pyroscope-io/pyroscope/commit/114eabd))
* Update deployment.svg ([b727bff](https://github.com/pyroscope-io/pyroscope/commit/b727bff))
* Update README.ch.md ([f16b131](https://github.com/pyroscope-io/pyroscope/commit/f16b131))
* Update readmes ([6ed27ce](https://github.com/pyroscope-io/pyroscope/commit/6ed27ce))
* updates dotnet examples docker image ([f445381](https://github.com/pyroscope-io/pyroscope/commit/f445381))
* Windows support (agent) (#212) ([c1babda](https://github.com/pyroscope-io/pyroscope/commit/c1babda)), closes [#212](https://github.com/pyroscope-io/pyroscope/issues/212)
* Closes: #192 Use the specific config structure for each command (#208) ([2f33d66](https://github.com/pyroscope-io/pyroscope/commit/2f33d66)), closes [#192](https://github.com/pyroscope-io/pyroscope/issues/192) [#208](https://github.com/pyroscope-io/pyroscope/issues/208)
* Closes: Feature: add ability to delete apps #119 (#221) ([2cc5f0e](https://github.com/pyroscope-io/pyroscope/commit/2cc5f0e)), closes [#119](https://github.com/pyroscope-io/pyroscope/issues/119) [#221](https://github.com/pyroscope-io/pyroscope/issues/221) [#119](https://github.com/pyroscope-io/pyroscope/issues/119)
* benchmark: brings back grafana config ([99c560a](https://github.com/pyroscope-io/pyroscope/commit/99c560a))



## <small>0.0.32 (2021-05-21)</small>

* Add .Net support (#200) ([99f1149](https://github.com/pyroscope-io/pyroscope/commit/99f1149)), closes [#200](https://github.com/pyroscope-io/pyroscope/issues/200)
* examples fix ([b4eb2eb](https://github.com/pyroscope-io/pyroscope/commit/b4eb2eb))
* Fix dotnetspy panic on premature exit and add tests. (#203) ([b2fff2b](https://github.com/pyroscope-io/pyroscope/commit/b2fff2b)), closes [#203](https://github.com/pyroscope-io/pyroscope/issues/203)
* fix the nil reference issue, just add the error handle logic (#194) ([fc9e25b](https://github.com/pyroscope-io/pyroscope/commit/fc9e25b)), closes [#194](https://github.com/pyroscope-io/pyroscope/issues/194)
* improves multi-platform support in dotnet examples ([0e8d6ff](https://github.com/pyroscope-io/pyroscope/commit/0e8d6ff))
* Update deployment diagram ([bc5d5e7](https://github.com/pyroscope-io/pyroscope/commit/bc5d5e7))
* Update deployment diagram ([fbd3a59](https://github.com/pyroscope-io/pyroscope/commit/fbd3a59))
* Update storage-design-ch.md ([e39332a](https://github.com/pyroscope-io/pyroscope/commit/e39332a))
* Update storage-design.md ([659c5c4](https://github.com/pyroscope-io/pyroscope/commit/659c5c4))
* Updates README and adds Credits section ([7773f33](https://github.com/pyroscope-io/pyroscope/commit/7773f33))



## <small>0.0.31 (2021-05-17)</small>

* Add roadmap to readme ([5372d78](https://github.com/pyroscope-io/pyroscope/commit/5372d78))
* added chinese translation for storage-design (#180) ([fbeb788](https://github.com/pyroscope-io/pyroscope/commit/fbeb788)), closes [#180](https://github.com/pyroscope-io/pyroscope/issues/180)
* Adds PHP support via phpspy (#157) ([16f2bff](https://github.com/pyroscope-io/pyroscope/commit/16f2bff)), closes [#157](https://github.com/pyroscope-io/pyroscope/issues/157)
* Cleans up pkg/storage/segment and global state (#183) ([fc51463](https://github.com/pyroscope-io/pyroscope/commit/fc51463)), closes [#183](https://github.com/pyroscope-io/pyroscope/issues/183)
* deletes an old unused script ([a91d465](https://github.com/pyroscope-io/pyroscope/commit/a91d465))
* Resolves #165 - implemented out of space check (#174) ([3628793](https://github.com/pyroscope-io/pyroscope/commit/3628793)), closes [#165](https://github.com/pyroscope-io/pyroscope/issues/165) [#174](https://github.com/pyroscope-io/pyroscope/issues/174)
* Update storage-design.md ([f2b83ae](https://github.com/pyroscope-io/pyroscope/commit/f2b83ae))
* updates contributors ([81148ef](https://github.com/pyroscope-io/pyroscope/commit/81148ef))



## <small>0.0.30 (2021-04-26)</small>

* adds buildx as a builder ([6234773](https://github.com/pyroscope-io/pyroscope/commit/6234773))
* build fix ([c328dbf](https://github.com/pyroscope-io/pyroscope/commit/c328dbf))
* controller code improvements ([3116e37](https://github.com/pyroscope-io/pyroscope/commit/3116e37))
* fix upload panic(do upload with recover) (#151) ([e73031a](https://github.com/pyroscope-io/pyroscope/commit/e73031a)), closes [#151](https://github.com/pyroscope-io/pyroscope/issues/151)
* flaky test fix ([e098d77](https://github.com/pyroscope-io/pyroscope/commit/e098d77))
* flaky test fix ([7afecca](https://github.com/pyroscope-io/pyroscope/commit/7afecca))
* github actions: prints available platforms ([ba443f9](https://github.com/pyroscope-io/pyroscope/commit/ba443f9))
* Golang Profiler — adds memory profiling, improves cpu profiling (#146) ([704fcfb](https://github.com/pyroscope-io/pyroscope/commit/704fcfb)), closes [#146](https://github.com/pyroscope-io/pyroscope/issues/146)
* improves support for multiple paths in GOPATH - fixes #142 ([5e06042](https://github.com/pyroscope-io/pyroscope/commit/5e06042)), closes [#142](https://github.com/pyroscope-io/pyroscope/issues/142)
* improves test descriptions (Describe / Context / It) ([35b4dca](https://github.com/pyroscope-io/pyroscope/commit/35b4dca))
* ran go mod tidy ([5274dd3](https://github.com/pyroscope-io/pyroscope/commit/5274dd3))
* removes a useless warning ([c50a77b](https://github.com/pyroscope-io/pyroscope/commit/c50a77b))
* removes dockerhub action ([c5079c9](https://github.com/pyroscope-io/pyroscope/commit/c5079c9))
* sets up docker builds from github actions to dockerhub ([d500f8d](https://github.com/pyroscope-io/pyroscope/commit/d500f8d))
* tests improvements ([74c5237](https://github.com/pyroscope-io/pyroscope/commit/74c5237))
* Update README.ch.md (#159) ([e711418](https://github.com/pyroscope-io/pyroscope/commit/e711418)), closes [#159](https://github.com/pyroscope-io/pyroscope/issues/159)
* updates README ([f20da5f](https://github.com/pyroscope-io/pyroscope/commit/f20da5f))
* Use bash to run scripts ([a019ef3](https://github.com/pyroscope-io/pyroscope/commit/a019ef3))



## <small>0.0.29 (2021-04-06)</small>

* Add node_modules, git folders to dockerignore ([306c5c4](https://github.com/pyroscope-io/pyroscope/commit/306c5c4))
* add parser type for ParseKey switch case ([f74af6f](https://github.com/pyroscope-io/pyroscope/commit/f74af6f))
* add strings.Builder.WriteString() method to whitelist in revive.toml ([c9da6d8](https://github.com/pyroscope-io/pyroscope/commit/c9da6d8))
* add tests for metadata serialization ([8e27013](https://github.com/pyroscope-io/pyroscope/commit/8e27013))
* Addressing #28 added some randomized tests ([8a7134c](https://github.com/pyroscope-io/pyroscope/commit/8a7134c)), closes [#28](https://github.com/pyroscope-io/pyroscope/issues/28)
* adds a github action to update contributors automatically (#128) ([a686fe7](https://github.com/pyroscope-io/pyroscope/commit/a686fe7)), closes [#128](https://github.com/pyroscope-io/pyroscope/issues/128)
* adds a lint rule for byte arrays ([b41c36e](https://github.com/pyroscope-io/pyroscope/commit/b41c36e))
* adds a very basic cli test ([187788e](https://github.com/pyroscope-io/pyroscope/commit/187788e))
* adds ability to specify user in pyroscope exec ([2bda8b1](https://github.com/pyroscope-io/pyroscope/commit/2bda8b1))
* adds basic tests for upstream/remote ([9bf67c6](https://github.com/pyroscope-io/pyroscope/commit/9bf67c6))
* adds cache tests ([fb42525](https://github.com/pyroscope-io/pyroscope/commit/fb42525))
* adds dictionary serialization tests ([98d0f59](https://github.com/pyroscope-io/pyroscope/commit/98d0f59))
* Adds ginkgo bootstrap files to all packages we should test (#126) ([f399f08](https://github.com/pyroscope-io/pyroscope/commit/f399f08)), closes [#126](https://github.com/pyroscope-io/pyroscope/issues/126)
* Adds Go Report to README ([a94aca3](https://github.com/pyroscope-io/pyroscope/commit/a94aca3))
* adds go tests ([886479a](https://github.com/pyroscope-io/pyroscope/commit/886479a))
* adds profiling data parser tests ([c4d60fd](https://github.com/pyroscope-io/pyroscope/commit/c4d60fd))
* adds smoke tests to storage ([466cd8b](https://github.com/pyroscope-io/pyroscope/commit/466cd8b))
* adds tests for agent/session.go ([0586d24](https://github.com/pyroscope-io/pyroscope/commit/0586d24))
* adds tests for attime ([bd69e65](https://github.com/pyroscope-io/pyroscope/commit/bd69e65))
* adds tests for storage/segment/timeline.go ([2bd256b](https://github.com/pyroscope-io/pyroscope/commit/2bd256b))
* adds tests for storage/tree ([bccbf93](https://github.com/pyroscope-io/pyroscope/commit/bccbf93))
* Allow enabling Badger's truncate option (#148) ([73e68a3](https://github.com/pyroscope-io/pyroscope/commit/73e68a3)), closes [#148](https://github.com/pyroscope-io/pyroscope/issues/148)
* better names ([4209607](https://github.com/pyroscope-io/pyroscope/commit/4209607))
* break down ParseKey's nested switch statement into separate function calls ([a586929](https://github.com/pyroscope-io/pyroscope/commit/a586929))
* bug fix ([cf12eec](https://github.com/pyroscope-io/pyroscope/commit/cf12eec))
* bug fix ([571df5d](https://github.com/pyroscope-io/pyroscope/commit/571df5d))
* changes badger-log-truncate to be on by default as discussed in #148 ([dc2a22c](https://github.com/pyroscope-io/pyroscope/commit/dc2a22c)), closes [#148](https://github.com/pyroscope-io/pyroscope/issues/148)
* Controller Improvements (#144) ([ad4ad80](https://github.com/pyroscope-io/pyroscope/commit/ad4ad80)), closes [#144](https://github.com/pyroscope-io/pyroscope/issues/144)
* exec tests ([a888823](https://github.com/pyroscope-io/pyroscope/commit/a888823))
* Export flamegraph and table to pdf/png (#143) ([7782248](https://github.com/pyroscope-io/pyroscope/commit/7782248)), closes [#143](https://github.com/pyroscope-io/pyroscope/issues/143)
* Fix Python3 Indentation - Following PEP8 style (#138) ([bd13663](https://github.com/pyroscope-io/pyroscope/commit/bd13663)), closes [#138](https://github.com/pyroscope-io/pyroscope/issues/138)
* Fix some Revive lint warnings (#145) ([d7ccae6](https://github.com/pyroscope-io/pyroscope/commit/d7ccae6)), closes [#145](https://github.com/pyroscope-io/pyroscope/issues/145)
* Fix typo in README (#124) ([f956810](https://github.com/pyroscope-io/pyroscope/commit/f956810)), closes [#124](https://github.com/pyroscope-io/pyroscope/issues/124)
* Fix typo in storage-design.md ([3638a0c](https://github.com/pyroscope-io/pyroscope/commit/3638a0c))
* fixes typos ([e1c6cad](https://github.com/pyroscope-io/pyroscope/commit/e1c6cad))
* improves test reliability ([c38e151](https://github.com/pyroscope-io/pyroscope/commit/c38e151))
* makes docker builds more resilient ([98284f7](https://github.com/pyroscope-io/pyroscope/commit/98284f7))
* refactors config initialization in tests ([988514d](https://github.com/pyroscope-io/pyroscope/commit/988514d))
* remove broken upload command ([58e292f](https://github.com/pyroscope-io/pyroscope/commit/58e292f))
* Small improvements to Dockerfile ([b3aaa29](https://github.com/pyroscope-io/pyroscope/commit/b3aaa29))
* splits test workflows ([4ddcfab](https://github.com/pyroscope-io/pyroscope/commit/4ddcfab))
* tests for analytics service ([94004d0](https://github.com/pyroscope-io/pyroscope/commit/94004d0))
* Typo fix ([d173be8](https://github.com/pyroscope-io/pyroscope/commit/d173be8))
* Update README.ch.md ([4c4dc1c](https://github.com/pyroscope-io/pyroscope/commit/4c4dc1c))
* Update storage-design.md (#135) ([3cd6242](https://github.com/pyroscope-io/pyroscope/commit/3cd6242)), closes [#135](https://github.com/pyroscope-io/pyroscope/issues/135)
* updates badges on README page ([f8105fd](https://github.com/pyroscope-io/pyroscope/commit/f8105fd))
* version 0.0.28 ([1fc03bb](https://github.com/pyroscope-io/pyroscope/commit/1fc03bb))
* wrap the strings.Builder WriteString calls in a separate function to handle error check ([66b1bb3](https://github.com/pyroscope-io/pyroscope/commit/66b1bb3))
* pyspy: defaults to non-blocking mode, adds an option to enable blocking mode ([0035a99](https://github.com/pyroscope-io/pyroscope/commit/0035a99))
* chore(style): update go format style. (#129) ([a7b15ca](https://github.com/pyroscope-io/pyroscope/commit/a7b15ca)), closes [#129](https://github.com/pyroscope-io/pyroscope/issues/129)



## <small>0.0.28 (2021-03-11)</small>

* adds a mutex to hyperloglog library, addresses #112 ([66ad877](https://github.com/pyroscope-io/pyroscope/commit/66ad877)), closes [#112](https://github.com/pyroscope-io/pyroscope/issues/112)
* adds codecov ([0b35827](https://github.com/pyroscope-io/pyroscope/commit/0b35827))
* adds storage design doc ([99aa4ad](https://github.com/pyroscope-io/pyroscope/commit/99aa4ad))
* exclude vendor directory from being linted.  Update ApiBindAddr to APIBindAddr ([27c33d2](https://github.com/pyroscope-io/pyroscope/commit/27c33d2))
* fix malformed struct tags ([a2d577d](https://github.com/pyroscope-io/pyroscope/commit/a2d577d))
* fixes syntax error ([13f5168](https://github.com/pyroscope-io/pyroscope/commit/13f5168))
* improves ebpf support on debian ([38918a1](https://github.com/pyroscope-io/pyroscope/commit/38918a1))
* increases maximum nodes per tree when rendering trees ([76d3fa2](https://github.com/pyroscope-io/pyroscope/commit/76d3fa2))
* makes logging less verbose, fixes a bug with direct upstream ([9784bc2](https://github.com/pyroscope-io/pyroscope/commit/9784bc2))
* remove naked return and cut down line length ([e7df6f6](https://github.com/pyroscope-io/pyroscope/commit/e7df6f6))
* removes mdx parts ([5b515e6](https://github.com/pyroscope-io/pyroscope/commit/5b515e6))
* spelling fixes ([eb08802](https://github.com/pyroscope-io/pyroscope/commit/eb08802))
* update typos ([e813976](https://github.com/pyroscope-io/pyroscope/commit/e813976))
* updates the list of contributors ([bc1b5d6](https://github.com/pyroscope-io/pyroscope/commit/bc1b5d6))
* version 0.0.27 ([28f31b8](https://github.com/pyroscope-io/pyroscope/commit/28f31b8))



## <small>0.0.27 (2021-03-10)</small>

* adds a mutex to hyperloglog library, addresses #112 ([66ad877](https://github.com/pyroscope-io/pyroscope/commit/66ad877)), closes [#112](https://github.com/pyroscope-io/pyroscope/issues/112)
* exclude vendor directory from being linted.  Update ApiBindAddr to APIBindAddr ([27c33d2](https://github.com/pyroscope-io/pyroscope/commit/27c33d2))
* fix malformed struct tags ([a2d577d](https://github.com/pyroscope-io/pyroscope/commit/a2d577d))
* remove naked return and cut down line length ([e7df6f6](https://github.com/pyroscope-io/pyroscope/commit/e7df6f6))
* updates the list of contributors ([bc1b5d6](https://github.com/pyroscope-io/pyroscope/commit/bc1b5d6))



## <small>0.0.26 (2021-03-08)</small>

* Add Pyroscope cloud instructions ([3765ea2](https://github.com/pyroscope-io/pyroscope/commit/3765ea2))
* add unit tests (#98) ([31474b3](https://github.com/pyroscope-io/pyroscope/commit/31474b3)), closes [#98](https://github.com/pyroscope-io/pyroscope/issues/98)
* check to see if logrus is used ([e2ff648](https://github.com/pyroscope-io/pyroscope/commit/e2ff648))
* fixes dependency graph task ([e4c2168](https://github.com/pyroscope-io/pyroscope/commit/e4c2168))
* removes logrus dependency from the library code ([f8e033e](https://github.com/pyroscope-io/pyroscope/commit/f8e033e))
* Update contributors ([a39c7ab](https://github.com/pyroscope-io/pyroscope/commit/a39c7ab))
* Updates README with information about the cloud ([2da1333](https://github.com/pyroscope-io/pyroscope/commit/2da1333))
* consistency: removes "log" alias in favor of "logrus" ([a66f8b3](https://github.com/pyroscope-io/pyroscope/commit/a66f8b3))



## <small>0.0.25 (2021-02-26)</small>

* adds eBPF spy ([3438826](https://github.com/pyroscope-io/pyroscope/commit/3438826))
* new connect command ([7d2a410](https://github.com/pyroscope-io/pyroscope/commit/7d2a410))
* update ignore (#93) ([2176676](https://github.com/pyroscope-io/pyroscope/commit/2176676)), closes [#93](https://github.com/pyroscope-io/pyroscope/issues/93)
* updates readme ([ce3f843](https://github.com/pyroscope-io/pyroscope/commit/ce3f843))



## <small>0.0.24 (2021-02-23)</small>

* Add comparison view (#90) ([bc648e9](https://github.com/pyroscope-io/pyroscope/commit/bc648e9)), closes [#90](https://github.com/pyroscope-io/pyroscope/issues/90)
* Add React Router (#73) ([1443b8c](https://github.com/pyroscope-io/pyroscope/commit/1443b8c)), closes [#73](https://github.com/pyroscope-io/pyroscope/issues/73)
* added chinese for flamgrahs (#89) ([11a22e6](https://github.com/pyroscope-io/pyroscope/commit/11a22e6)), closes [#89](https://github.com/pyroscope-io/pyroscope/issues/89)
* fix connection reuse (#87) ([8fb42ec](https://github.com/pyroscope-io/pyroscope/commit/8fb42ec)), closes [#87](https://github.com/pyroscope-io/pyroscope/issues/87)
* fixes air config (`make dev` command) ([3a3b366](https://github.com/pyroscope-io/pyroscope/commit/3a3b366))
* fixes ruby stacktraces formatting ([4a7c2be](https://github.com/pyroscope-io/pyroscope/commit/4a7c2be))
* Import pyroscope version from package.json (#86) ([295a002](https://github.com/pyroscope-io/pyroscope/commit/295a002)), closes [#86](https://github.com/pyroscope-io/pyroscope/issues/86)
* Move flamebearer to local state instead of redux store (#77) ([03f3abc](https://github.com/pyroscope-io/pyroscope/commit/03f3abc)), closes [#77](https://github.com/pyroscope-io/pyroscope/issues/77)
* refresh button fix ([649678e](https://github.com/pyroscope-io/pyroscope/commit/649678e))
* Update contributors ([10e1740](https://github.com/pyroscope-io/pyroscope/commit/10e1740))
* cleanup: lint js code (#80) ([3dab5d9](https://github.com/pyroscope-io/pyroscope/commit/3dab5d9)), closes [#80](https://github.com/pyroscope-io/pyroscope/issues/80)



## <small>0.0.23 (2021-02-17)</small>

* Add translations structure (#81) ([05b9fa8](https://github.com/pyroscope-io/pyroscope/commit/05b9fa8)), closes [#81](https://github.com/pyroscope-io/pyroscope/issues/81)
* Create README.ch.md ([fe33f46](https://github.com/pyroscope-io/pyroscope/commit/fe33f46))
* Update diagram in CH version ([72d98c6](https://github.com/pyroscope-io/pyroscope/commit/72d98c6))
* updates rbspy version (relative filenames update) ([a58d354](https://github.com/pyroscope-io/pyroscope/commit/a58d354))



## <small>0.0.22 (2021-02-16)</small>

* fix upload URL to include existing path (#76) ([221988f](https://github.com/pyroscope-io/pyroscope/commit/221988f)), closes [#76](https://github.com/pyroscope-io/pyroscope/issues/76)
* makes controller fail more gracefully when the db is closing ([ae5cc0f](https://github.com/pyroscope-io/pyroscope/commit/ae5cc0f))
* updates contributors ([337cc12](https://github.com/pyroscope-io/pyroscope/commit/337cc12))
* updates rbspy version, adds comments to rbspy and pyspy ([cbc5a7b](https://github.com/pyroscope-io/pyroscope/commit/cbc5a7b))
* fix: unknown revision cosmtrek/air v1.21.2 => v1.12.2 (#75) ([9eaed48](https://github.com/pyroscope-io/pyroscope/commit/9eaed48)), closes [#75](https://github.com/pyroscope-io/pyroscope/issues/75)



## <small>0.0.21 (2021-02-13)</small>

* adds ARCHITECTURE.md file ([658410d](https://github.com/pyroscope-io/pyroscope/commit/658410d))
* adds comparison view button ([c6ee9e0](https://github.com/pyroscope-io/pyroscope/commit/c6ee9e0))
* adds order to config fields, adds a sample config generator program, fixes #66 ([0569dc6](https://github.com/pyroscope-io/pyroscope/commit/0569dc6)), closes [#66](https://github.com/pyroscope-io/pyroscope/issues/66)
* improves readme with simpler instructions ([a059050](https://github.com/pyroscope-io/pyroscope/commit/a059050))
* simplifies python example ([a0e3cb5](https://github.com/pyroscope-io/pyroscope/commit/a0e3cb5))
* styling fixes ([9fee8ae](https://github.com/pyroscope-io/pyroscope/commit/9fee8ae))
* updates language names ([a1447dd](https://github.com/pyroscope-io/pyroscope/commit/a1447dd))
* updates README with 2 step install process ([70954b9](https://github.com/pyroscope-io/pyroscope/commit/70954b9))
* updates README with an updated diagram ([8f85bb4](https://github.com/pyroscope-io/pyroscope/commit/8f85bb4))
* frontend: adds a sidebar, simplifies header ([2cdf22a](https://github.com/pyroscope-io/pyroscope/commit/2cdf22a))



## <small>0.0.20 (2021-02-08)</small>

* fixes darwin build ([4907ade](https://github.com/pyroscope-io/pyroscope/commit/4907ade))
* new util to convert from and until values to date Object ([1e758cf](https://github.com/pyroscope-io/pyroscope/commit/1e758cf))
* renamed formatAsOBject() parameter ([874941b](https://github.com/pyroscope-io/pyroscope/commit/874941b))
* Updated to use the new util formatAsOBject() ([b59744a](https://github.com/pyroscope-io/pyroscope/commit/b59744a))



## <small>0.0.19 (2021-02-08)</small>

* adds config option for setting the app's base url (#69) ([86f07d7](https://github.com/pyroscope-io/pyroscope/commit/86f07d7)), closes [#69](https://github.com/pyroscope-io/pyroscope/issues/69)
* explicit error message when used with our hosted version and no auth-token provided ([d6d9b45](https://github.com/pyroscope-io/pyroscope/commit/d6d9b45))
* improves Usage message ([d2d4535](https://github.com/pyroscope-io/pyroscope/commit/d2d4535))
* linux, exec: adds capabilities detection - prints a message when running pyroscope exec with no prop ([e37a9a0](https://github.com/pyroscope-io/pyroscope/commit/e37a9a0))
* makes `pyroscope exec help` print usage message ([dfce70c](https://github.com/pyroscope-io/pyroscope/commit/dfce70c))
* makes favicon url relative ([c176b73](https://github.com/pyroscope-io/pyroscope/commit/c176b73))
* updates rbspy / pyspy version, enables arm builds ([30ec4c0](https://github.com/pyroscope-io/pyroscope/commit/30ec4c0))
* updates the list of contributors ([6137fa2](https://github.com/pyroscope-io/pyroscope/commit/6137fa2))



## <small>0.0.18 (2021-02-04)</small>

* Add FlameGraphRendererNew, factor out ProfilerHeader and ProfilerTable ([5c0f137](https://github.com/pyroscope-io/pyroscope/commit/5c0f137))
* Added a date and time picker ([7246d32](https://github.com/pyroscope-io/pyroscope/commit/7246d32))
* adds 'gospy.(*GoSpy).Snapshot' to list of excludes in gospy, addresses #50 ([a565c52](https://github.com/pyroscope-io/pyroscope/commit/a565c52)), closes [#50](https://github.com/pyroscope-io/pyroscope/issues/50)
* adds a lint comment ([8da5bae](https://github.com/pyroscope-io/pyroscope/commit/8da5bae))
* adds a revive exception for code that we didn't write ([0616b14](https://github.com/pyroscope-io/pyroscope/commit/0616b14))
* adds auto-review github action ([da0f556](https://github.com/pyroscope-io/pyroscope/commit/da0f556))
* auto review config improvements ([bc5eaa5](https://github.com/pyroscope-io/pyroscope/commit/bc5eaa5))
* Change cursor to pointer, change crosshair color ([1b9c1b4](https://github.com/pyroscope-io/pyroscope/commit/1b9c1b4))
* changed date format to YYYY-mm-dd HH:MM ([609d2e4](https://github.com/pyroscope-io/pyroscope/commit/609d2e4))
* coloring date as blue when it's part of the range ([a9155d9](https://github.com/pyroscope-io/pyroscope/commit/a9155d9))
* Fix ProfilerHeader lint ([2f987ce](https://github.com/pyroscope-io/pyroscope/commit/2f987ce))
* fix typo ([67f1567](https://github.com/pyroscope-io/pyroscope/commit/67f1567))
* Fix Update button from falling out, add date range labels ([534ce1f](https://github.com/pyroscope-io/pyroscope/commit/534ce1f))
* fixes an issue with forks and "Auto Request Review" ([9880432](https://github.com/pyroscope-io/pyroscope/commit/9880432))
* fixes work function ([00753b7](https://github.com/pyroscope-io/pyroscope/commit/00753b7))
* from selector fixed ([ed7e47b](https://github.com/pyroscope-io/pyroscope/commit/ed7e47b))
* Import crosshair library, define function that draws crosshair ([383fcd2](https://github.com/pyroscope-io/pyroscope/commit/383fcd2))
* improved the date range selector and fixed #54 #60 ([6194b88](https://github.com/pyroscope-io/pyroscope/commit/6194b88)), closes [#54](https://github.com/pyroscope-io/pyroscope/issues/54) [#60](https://github.com/pyroscope-io/pyroscope/issues/60)
* makes exec work with auth token ([9039949](https://github.com/pyroscope-io/pyroscope/commit/9039949))
* Merge FlameGraphRendererNew to FlameGraphRenderer ([a235867](https://github.com/pyroscope-io/pyroscope/commit/a235867))
* removed coloring range on hover ([c20d909](https://github.com/pyroscope-io/pyroscope/commit/c20d909))
* renames errFoo -> errClosing ([aaf0a68](https://github.com/pyroscope-io/pyroscope/commit/aaf0a68))
* reverts examples/golang/main.go changes ([9fa9e34](https://github.com/pyroscope-io/pyroscope/commit/9fa9e34))
* Set cross hair settings ([7bf4ab5](https://github.com/pyroscope-io/pyroscope/commit/7bf4ab5))
* Small css changes ([f70da15](https://github.com/pyroscope-io/pyroscope/commit/f70da15))
* split the code into reusable pieces, fixed issue#55 ([2c81645](https://github.com/pyroscope-io/pyroscope/commit/2c81645)), closes [issue#55](https://github.com/pyroscope-io/pyroscope/issues/55)
* support for auth tokens, atexit fix ([ff206a3](https://github.com/pyroscope-io/pyroscope/commit/ff206a3))
* trying to add auto-labeler + fix auto-review actionAuto label (#63) ([bc7aa2f](https://github.com/pyroscope-io/pyroscope/commit/bc7aa2f)), closes [#63](https://github.com/pyroscope-io/pyroscope/issues/63)
* updated ([320ac81](https://github.com/pyroscope-io/pyroscope/commit/320ac81))
* updated 2/ ([9734d74](https://github.com/pyroscope-io/pyroscope/commit/9734d74))
* updates our list of contributors ([0dc9715](https://github.com/pyroscope-io/pyroscope/commit/0dc9715))
* updates the list of contributors ([032c148](https://github.com/pyroscope-io/pyroscope/commit/032c148))
* cleanup: lint go code ([04c69b1](https://github.com/pyroscope-io/pyroscope/commit/04c69b1))



## <small>0.0.17 (2021-01-27)</small>

* Add logo to examples page ([1ea5655](https://github.com/pyroscope-io/pyroscope/commit/1ea5655))
* Add more examples tweaks ([d1f4f70](https://github.com/pyroscope-io/pyroscope/commit/d1f4f70))
* Add readme for how to debug python ([c0da18e](https://github.com/pyroscope-io/pyroscope/commit/c0da18e))
* adds build info to main page ([1133949](https://github.com/pyroscope-io/pyroscope/commit/1133949))
* better handling of application names ([59266bc](https://github.com/pyroscope-io/pyroscope/commit/59266bc))
* Change files for blog example ([3f9f55b](https://github.com/pyroscope-io/pyroscope/commit/3f9f55b))
* changes from time to iterations ([8e39892](https://github.com/pyroscope-io/pyroscope/commit/8e39892))
* Custom date range now no-ops ([4e31536](https://github.com/pyroscope-io/pyroscope/commit/4e31536))
* Enable lint JSX ([8e2d3a4](https://github.com/pyroscope-io/pyroscope/commit/8e2d3a4))
* first steps towards a dbmanager command ([dbe6e1d](https://github.com/pyroscope-io/pyroscope/commit/dbe6e1d))
* Fix Custom Date Range Input ([f316a17](https://github.com/pyroscope-io/pyroscope/commit/f316a17))
* Fix link in readme ([b47420f](https://github.com/pyroscope-io/pyroscope/commit/b47420f))
* Fix refresh button ([dabc101](https://github.com/pyroscope-io/pyroscope/commit/dabc101))
* Fix timeline and useEffect usage ([bffb8b3](https://github.com/pyroscope-io/pyroscope/commit/bffb8b3))
* fix typo in readme ([9399c28](https://github.com/pyroscope-io/pyroscope/commit/9399c28))
* implements permissions drop in pyroscope exec ([08948ef](https://github.com/pyroscope-io/pyroscope/commit/08948ef))
* improves rbspy + pyspy integrations, resolves #5 ([6ea7bcf](https://github.com/pyroscope-io/pyroscope/commit/6ea7bcf)), closes [#5](https://github.com/pyroscope-io/pyroscope/issues/5)
* Lint & Modernize DateRangePicker.jsx ([1c0021d](https://github.com/pyroscope-io/pyroscope/commit/1c0021d))
* Lint & Modernize DownloadButton.jsx ([a903e0a](https://github.com/pyroscope-io/pyroscope/commit/a903e0a))
* Lint & Modernize Footer.jsx ([30d6336](https://github.com/pyroscope-io/pyroscope/commit/30d6336))
* Lint & Modernize Header.jsx ([925f8a0](https://github.com/pyroscope-io/pyroscope/commit/925f8a0))
* Lint & Modernize Label.jsx ([0f225ce](https://github.com/pyroscope-io/pyroscope/commit/0f225ce))
* Lint & Modernize LabelsFilter.jsx ([3ffc005](https://github.com/pyroscope-io/pyroscope/commit/3ffc005))
* Lint & Modernize NameSelector.jsx ([fb27aea](https://github.com/pyroscope-io/pyroscope/commit/fb27aea))
* Lint & Modernize PyroscopeApp.jsx ([68f46ef](https://github.com/pyroscope-io/pyroscope/commit/68f46ef))
* Lint & Modernize RefreshButton.jsx ([05fd92b](https://github.com/pyroscope-io/pyroscope/commit/05fd92b))
* Lint & Modernize ShortcutsModal ([8fba659](https://github.com/pyroscope-io/pyroscope/commit/8fba659))
* Lint & Modernize ZoomOutButton.jsx ([9bee480](https://github.com/pyroscope-io/pyroscope/commit/9bee480))
* lint fix ([7cb7932](https://github.com/pyroscope-io/pyroscope/commit/7cb7932))
* Lint index.jsx ([cdc73bd](https://github.com/pyroscope-io/pyroscope/commit/cdc73bd))
* Lint MaxNodesSelector.jsx ([ae6f34d](https://github.com/pyroscope-io/pyroscope/commit/ae6f34d))
* Lint SlackIcon.jsx ([ff318c8](https://github.com/pyroscope-io/pyroscope/commit/ff318c8))
* Lint TimelineChart.jsx ([4d05072](https://github.com/pyroscope-io/pyroscope/commit/4d05072))
* Make demo link bigger in readme ([2897d33](https://github.com/pyroscope-io/pyroscope/commit/2897d33))
* Make picture in docs smaller and label it ([2b4bdfc](https://github.com/pyroscope-io/pyroscope/commit/2b4bdfc))
* rbspy update (brings ruby 2.7.2 support) ([89d974b](https://github.com/pyroscope-io/pyroscope/commit/89d974b))
* Rename python debugging file ([38d8221](https://github.com/pyroscope-io/pyroscope/commit/38d8221))
* time.Time support in CLI arguments ([8f47c7d](https://github.com/pyroscope-io/pyroscope/commit/8f47c7d))
* Tweak Readme ([46c4f3f](https://github.com/pyroscope-io/pyroscope/commit/46c4f3f))
* Update colors ([d0522ae](https://github.com/pyroscope-io/pyroscope/commit/d0522ae))
* Update examples readme to reference new guid ([351969a](https://github.com/pyroscope-io/pyroscope/commit/351969a))
* Update how_to_debug_python.md ([ad6f513](https://github.com/pyroscope-io/pyroscope/commit/ad6f513))
* Update python example using iterations ([c7e2298](https://github.com/pyroscope-io/pyroscope/commit/c7e2298))
* Update readme with longer example ([8cf44c4](https://github.com/pyroscope-io/pyroscope/commit/8cf44c4))
* working dbmanager command ([e7b9a68](https://github.com/pyroscope-io/pyroscope/commit/e7b9a68))



## <small>0.0.16 (2021-01-22)</small>

* Add node.js to upcoming integrations ([ad95f96](https://github.com/pyroscope-io/pyroscope/commit/ad95f96))
* brings back hide-applications option ([63693c1](https://github.com/pyroscope-io/pyroscope/commit/63693c1))
* changes ([a0cf5fd](https://github.com/pyroscope-io/pyroscope/commit/a0cf5fd))
* improves documentation for config options ([ec9a1f9](https://github.com/pyroscope-io/pyroscope/commit/ec9a1f9))
* improves exec child process handling ([ab18dd3](https://github.com/pyroscope-io/pyroscope/commit/ab18dd3))
* improves styles a little bit ([604ab00](https://github.com/pyroscope-io/pyroscope/commit/604ab00))
* Readme examples changes ([2105963](https://github.com/pyroscope-io/pyroscope/commit/2105963))
* update readme ([21f71e8](https://github.com/pyroscope-io/pyroscope/commit/21f71e8))
* Update Readme with new logo ([f3addba](https://github.com/pyroscope-io/pyroscope/commit/f3addba))
* updates our contributors list ([89349d6](https://github.com/pyroscope-io/pyroscope/commit/89349d6))



## <small>0.0.15 (2021-01-20)</small>

* implements #29 ([a42a241](https://github.com/pyroscope-io/pyroscope/commit/a42a241)), closes [#29](https://github.com/pyroscope-io/pyroscope/issues/29)
* new logo ([a22b529](https://github.com/pyroscope-io/pyroscope/commit/a22b529))



## <small>0.0.14 (2021-01-19)</small>

* fixes a bug where pyroscope thinks a spy is not supported when it is ([0a9da1c](https://github.com/pyroscope-io/pyroscope/commit/0a9da1c))



## <small>0.0.13 (2021-01-19)</small>

* Add a few more colors and remove extra comment ([44dfb62](https://github.com/pyroscope-io/pyroscope/commit/44dfb62))
* Add browser and node env to eslint config ([d0db6da](https://github.com/pyroscope-io/pyroscope/commit/d0db6da))
* Add eslint, prettier config ([ec55923](https://github.com/pyroscope-io/pyroscope/commit/ec55923))
* Add Label component test ([9878b18](https://github.com/pyroscope-io/pyroscope/commit/9878b18))
* Add lint to Github Actions ([af6b734](https://github.com/pyroscope-io/pyroscope/commit/af6b734))
* Add prettier and eslint config ([9ed1c6a](https://github.com/pyroscope-io/pyroscope/commit/9ed1c6a))
* Add testing libraries ([a3b46f2](https://github.com/pyroscope-io/pyroscope/commit/a3b46f2))
* Add webapp tests to github actions ([f2888bd](https://github.com/pyroscope-io/pyroscope/commit/f2888bd))
* adds contributors to our README ([ea8ca81](https://github.com/pyroscope-io/pyroscope/commit/ea8ca81))
* adds eslintcache to gitignore ([fc44cea](https://github.com/pyroscope-io/pyroscope/commit/fc44cea))
* adds v0.0.12 changelog ([774f0b5](https://github.com/pyroscope-io/pyroscope/commit/774f0b5))
* Auto Fix Linting Issues ([1f4e7b2](https://github.com/pyroscope-io/pyroscope/commit/1f4e7b2))
* changes the name of icicle button to flamegraph ([3917187](https://github.com/pyroscope-io/pyroscope/commit/3917187))
* Convert remaining lint errors to warning ([815cde2](https://github.com/pyroscope-io/pyroscope/commit/815cde2))
* disables commands that are not documented ([7c1ca7a](https://github.com/pyroscope-io/pyroscope/commit/7c1ca7a))
* exec improvements, extra messages for when agent can't start ([6af5383](https://github.com/pyroscope-io/pyroscope/commit/6af5383))
* Fix lint issues ([14ad03e](https://github.com/pyroscope-io/pyroscope/commit/14ad03e))
* fixes a bug in storage/segment get method ([47ebbd9](https://github.com/pyroscope-io/pyroscope/commit/47ebbd9))
* Fixes docker build ([06f2ca3](https://github.com/pyroscope-io/pyroscope/commit/06f2ca3))
* fixes makefile webapp cleanup ([58cf3e7](https://github.com/pyroscope-io/pyroscope/commit/58cf3e7))
* fixes wording ([a125021](https://github.com/pyroscope-io/pyroscope/commit/a125021))
* improves analytics ([c9423b8](https://github.com/pyroscope-io/pyroscope/commit/c9423b8))
* improves duration formatting ([c9b75d5](https://github.com/pyroscope-io/pyroscope/commit/c9b75d5))
* improves homebrew support ([a25b8ee](https://github.com/pyroscope-io/pyroscope/commit/a25b8ee))
* little cleanup ([4230692](https://github.com/pyroscope-io/pyroscope/commit/4230692))
* Migrate .babelrc to babel.config.js ([dd1c953](https://github.com/pyroscope-io/pyroscope/commit/dd1c953))
* removes binary file that should not have been commited in the first place ([a8f0f7c](https://github.com/pyroscope-io/pyroscope/commit/a8f0f7c))
* Removes scary language ([0ed1809](https://github.com/pyroscope-io/pyroscope/commit/0ed1809))
* renames babelrc to babel.config.js in  Dockerfile ([bddb968](https://github.com/pyroscope-io/pyroscope/commit/bddb968))
* Setup ESLint ([e737863](https://github.com/pyroscope-io/pyroscope/commit/e737863))
* Simplifies Dockerfile ([679206a](https://github.com/pyroscope-io/pyroscope/commit/679206a))
* storage/segment test fixes ([e40c02c](https://github.com/pyroscope-io/pyroscope/commit/e40c02c))
* Update main readme with diagram ([ee465d3](https://github.com/pyroscope-io/pyroscope/commit/ee465d3))
* Update tests.yml ([7fe6047](https://github.com/pyroscope-io/pyroscope/commit/7fe6047))
* updates README with a better demo gif ([591ce36](https://github.com/pyroscope-io/pyroscope/commit/591ce36))
* updates README with more install instructions ([da03ff5](https://github.com/pyroscope-io/pyroscope/commit/da03ff5))
* table: work in progress ([058e61f](https://github.com/pyroscope-io/pyroscope/commit/058e61f))
* table: work in progress ([75b4920](https://github.com/pyroscope-io/pyroscope/commit/75b4920))



## <small>0.0.12 (2021-01-11)</small>

* adds ability to hide certain apps on the frontend ([02a505f](https://github.com/pyroscope-io/pyroscope/commit/02a505f))
* adds docker-compose examples for python, ruby and go ([d0572a5](https://github.com/pyroscope-io/pyroscope/commit/d0572a5))
* adds more badges ([86c58ef](https://github.com/pyroscope-io/pyroscope/commit/86c58ef))
* adds version to copyright on hover ([b16e11e](https://github.com/pyroscope-io/pyroscope/commit/b16e11e))
* allows UI to resize below 1430px width ([e38f649](https://github.com/pyroscope-io/pyroscope/commit/e38f649))
* bumps up version to 0.0.12 ([21d8597](https://github.com/pyroscope-io/pyroscope/commit/21d8597))
* changes port from 8080 to 4040 ([cd36317](https://github.com/pyroscope-io/pyroscope/commit/cd36317))
* fixes the condition from > to >= for "other" blocks ([3e74f2c](https://github.com/pyroscope-io/pyroscope/commit/3e74f2c))
* updates README with a clearer value proposition ([58cdb4e](https://github.com/pyroscope-io/pyroscope/commit/58cdb4e))
* updates README with more links ([c20a00f](https://github.com/pyroscope-io/pyroscope/commit/c20a00f))
* docker: changes package-lock.json to yarn.lock ([f321857](https://github.com/pyroscope-io/pyroscope/commit/f321857))



## <small>0.0.11 (2021-01-03)</small>

* changelog: better format ([c5bbb68](https://github.com/pyroscope-io/pyroscope/commit/c5bbb68))
* adds a custom rbspy regex matcher, removes maxNodes selector ([86e70f4](https://github.com/pyroscope-io/pyroscope/commit/86e70f4))
* adds a demo link to README ([3a45564](https://github.com/pyroscope-io/pyroscope/commit/3a45564))
* adds a task to update changelog ([8c75689](https://github.com/pyroscope-io/pyroscope/commit/8c75689))
* adds conventional-changelog ([164f599](https://github.com/pyroscope-io/pyroscope/commit/164f599))
* adds version as one of the things we report in analytics ([e67a797](https://github.com/pyroscope-io/pyroscope/commit/e67a797))
* allows to add extra_metadata to <head> during docker build (this is for demo.pyroscope.io) ([6294eb7](https://github.com/pyroscope-io/pyroscope/commit/6294eb7))
* fixes pyroscope README ([2607a7f](https://github.com/pyroscope-io/pyroscope/commit/2607a7f))
* js lock changes ([864622f](https://github.com/pyroscope-io/pyroscope/commit/864622f))
* makes it so that we no longer drop root element for flamebearer ([4f4d46c](https://github.com/pyroscope-io/pyroscope/commit/4f4d46c))
* replaces "Metric" with a clearer "Application" ([eb496d1](https://github.com/pyroscope-io/pyroscope/commit/eb496d1))
* updates package.json version number ([9cc1728](https://github.com/pyroscope-io/pyroscope/commit/9cc1728))
* updates README ([374e216](https://github.com/pyroscope-io/pyroscope/commit/374e216))
* docs: makes demo gif a link ([dbc43bb](https://github.com/pyroscope-io/pyroscope/commit/dbc43bb))

