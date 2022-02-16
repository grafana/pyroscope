## [0.10.2](https://github.com/pyroscope-io/pyroscope/compare/v0.10.1...v0.10.2) (2022-02-15)


### Bug Fixes

* CORS allow credentials ([#846](https://github.com/pyroscope-io/pyroscope/issues/846)) ([3fef7a5](https://github.com/pyroscope-io/pyroscope/commit/3fef7a5fee7968c566d5fd2b1a7c85a209de815a))
* **frontend:** move CSS to css modules ([#842](https://github.com/pyroscope-io/pyroscope/issues/842)) ([3aadc13](https://github.com/pyroscope-io/pyroscope/commit/3aadc13749ed484f1167b858ded8aa8e56743c39))



## [0.10.1](https://github.com/pyroscope-io/pyroscope/compare/v0.10.0...v0.10.1) (2022-02-14)


### Bug Fixes

* adhoc comparison / diff routes. ([#834](https://github.com/pyroscope-io/pyroscope/issues/834)) ([1ab101c](https://github.com/pyroscope-io/pyroscope/commit/1ab101c20742d55c17b112f6d3eec942e96091bb))
* **frontend:** safari date-fns fix ([#838](https://github.com/pyroscope-io/pyroscope/issues/838)) ([896b936](https://github.com/pyroscope-io/pyroscope/commit/896b93605bd0b0190838b45a647f4689d93360ab))
* handle properly writing errors in adhoc mode. ([#833](https://github.com/pyroscope-io/pyroscope/issues/833)) ([8e77b8b](https://github.com/pyroscope-io/pyroscope/commit/8e77b8b627ad654f135a7f8c811bc411369a2518))



# [0.10.0](https://github.com/pyroscope-io/pyroscope/compare/v0.9.0...v0.10.0) (2022-02-14)


### Bug Fixes

* **frontend:** fix coloring for pull mode ([#822](https://github.com/pyroscope-io/pyroscope/issues/822)) ([a221400](https://github.com/pyroscope-io/pyroscope/commit/a221400027097ff33864f4cba0ff5bfde78295f1))
* **frontend:** quickfix for wierd dropdown behaviour ([#832](https://github.com/pyroscope-io/pyroscope/issues/832)) ([c6da525](https://github.com/pyroscope-io/pyroscope/commit/c6da525d30a4271fd5ca74e2c6bf35230042c47e))
* ignore root node when converting a flamebearer to a tree. ([#812](https://github.com/pyroscope-io/pyroscope/issues/812)) ([7751b15](https://github.com/pyroscope-io/pyroscope/commit/7751b1562194eafa21fc77e3601c4a525250c31c))
* store "total" in name cache, and make tests more rigurous. ([#821](https://github.com/pyroscope-io/pyroscope/issues/821)) ([e46f2cc](https://github.com/pyroscope-io/pyroscope/commit/e46f2ccc66fbd4b238b5f59629965156e7fe5dbb))


### Features

* add an optional name field to the profile data format. ([#826](https://github.com/pyroscope-io/pyroscope/issues/826)) ([26d8177](https://github.com/pyroscope-io/pyroscope/commit/26d817746078ffef2059560552cac0e45495182c))
* add upload support in adhoc server. ([#801](https://github.com/pyroscope-io/pyroscope/issues/801)) ([8551df1](https://github.com/pyroscope-io/pyroscope/commit/8551df1973f3dfacfda7d2b33055e994e5ffad57))
* **frontend:** export diff to flamegraph.com ([#808](https://github.com/pyroscope-io/pyroscope/issues/808)) ([a2e47b2](https://github.com/pyroscope-io/pyroscope/commit/a2e47b25646dc8bec386e1d98f9c87503c0ec0d2))
* identity and access management ([#739](https://github.com/pyroscope-io/pyroscope/issues/739)) ([0ca0d83](https://github.com/pyroscope-io/pyroscope/commit/0ca0d8398cbbc58799e0e53b658c70b8670c6e72)), closes [#770](https://github.com/pyroscope-io/pyroscope/issues/770) [#807](https://github.com/pyroscope-io/pyroscope/issues/807) [#814](https://github.com/pyroscope-io/pyroscope/issues/814)



# [0.9.0](https://github.com/pyroscope-io/pyroscope/compare/v0.8.0...v0.9.0) (2022-02-07)


### Bug Fixes

* Solve panic on an empty profile ingest ([#793](https://github.com/pyroscope-io/pyroscope/issues/793)) ([2d3a479](https://github.com/pyroscope-io/pyroscope/commit/2d3a479dfb8855c9bf75528bc46ee221a4a02cfe))
* unsafeStrToSlice panic due to empty string ([#772](https://github.com/pyroscope-io/pyroscope/issues/772)) ([189f775](https://github.com/pyroscope-io/pyroscope/commit/189f7753bae460bb170ae63c72645e6df7b8f2d3))


### Features

* add some basic flamebearer validation. ([#785](https://github.com/pyroscope-io/pyroscope/issues/785)) ([bee6483](https://github.com/pyroscope-io/pyroscope/commit/bee6483d88d7381ba778f50c557667f4eb1543eb))
* experimental tracing integration ([#766](https://github.com/pyroscope-io/pyroscope/issues/766)) ([24af197](https://github.com/pyroscope-io/pyroscope/commit/24af197b37c572425ae7896a0f446709bcdde4f1))
* **frontend:** add package coloring for rust ([#798](https://github.com/pyroscope-io/pyroscope/issues/798)) ([c687f83](https://github.com/pyroscope-io/pyroscope/commit/c687f8382e0b5c87dcc6fd1ac3ffdd2e11464952))
* **frontend:** adds ability to export to flamegraph.com ([#799](https://github.com/pyroscope-io/pyroscope/issues/799)) ([a3828bc](https://github.com/pyroscope-io/pyroscope/commit/a3828bc93fdf76478328f5c65c117b7737993e61))



# [0.8.0](https://github.com/pyroscope-io/pyroscope/compare/v0.7.2...v0.8.0) (2022-01-25)


### Bug Fixes

* **examples:** adds host pid option to docker-compose eBPF example([#732](https://github.com/pyroscope-io/pyroscope/issues/732)) ([5e8dc83](https://github.com/pyroscope-io/pyroscope/commit/5e8dc8359bb15ce8406c5a2cf4ceb1355a43be00))
* **frontend:** improves timeline UX by shifting bars 5 seconds forward ([#742](https://github.com/pyroscope-io/pyroscope/issues/742)) ([687219d](https://github.com/pyroscope-io/pyroscope/commit/687219d180224e4fd750dcd697396200f12bcac0))
* return disk space check ([#751](https://github.com/pyroscope-io/pyroscope/issues/751)) ([0641244](https://github.com/pyroscope-io/pyroscope/commit/0641244e18cdbe29c7044ce95c6e00acbab9b2a7))
* Update drag and drop styling ([#756](https://github.com/pyroscope-io/pyroscope/issues/756)) ([25ce3b2](https://github.com/pyroscope-io/pyroscope/commit/25ce3b2f52428043d04f152f67fa10fb3b118049))


### Features

* add debug storage export endpoint ([#752](https://github.com/pyroscope-io/pyroscope/issues/752)) ([5040fb3](https://github.com/pyroscope-io/pyroscope/commit/5040fb3a3b266eed4843ed4e8e5687dfcbd49189))
* add http discovery mechanism ([#726](https://github.com/pyroscope-io/pyroscope/issues/726)) ([a941634](https://github.com/pyroscope-io/pyroscope/commit/a94163423e3689979c1ace74fdfdc140d37d9713))
* added tooltip for timeline selection ([#730](https://github.com/pyroscope-io/pyroscope/issues/730)) ([d226370](https://github.com/pyroscope-io/pyroscope/commit/d226370239293dd14e01c907a471e68c8f915a2b))
* **frontend:** export comparison diff standalone html ([#749](https://github.com/pyroscope-io/pyroscope/issues/749)) ([697a66c](https://github.com/pyroscope-io/pyroscope/commit/697a66c925178de43d09d051f83a9dc0f39207a9))
* New diff mode palette selection dropdown ([#754](https://github.com/pyroscope-io/pyroscope/issues/754)) ([dfd8a3d](https://github.com/pyroscope-io/pyroscope/commit/dfd8a3d04900eadead8faf588cfa1d01bbf519b2))
* output standalone HTML files for adhoc profiles. ([#728](https://github.com/pyroscope-io/pyroscope/issues/728)) ([a4f90ab](https://github.com/pyroscope-io/pyroscope/commit/a4f90ab3cc6f5e4536db2f4fdbcb1d6bee5f790b))



## [0.7.2](https://github.com/pyroscope-io/pyroscope/compare/v0.7.1...v0.7.2) (2022-01-14)


### Features

* adds ability to trace storage.Get ([#731](https://github.com/pyroscope-io/pyroscope/issues/731)) ([9157e5e](https://github.com/pyroscope-io/pyroscope/commit/9157e5ec498a8b1a53c898da300cdbfa47d0fb4e))



## [0.7.1](https://github.com/pyroscope-io/pyroscope/compare/v0.7.0...v0.7.1) (2022-01-13)


### Bug Fixes

* **backend:** skip empty app with GetAppNames() ([#724](https://github.com/pyroscope-io/pyroscope/issues/724)) ([b3fadec](https://github.com/pyroscope-io/pyroscope/commit/b3fadeccee1fc7c865b3564d0ce27663e66cd7f5))
* **frontend:** don't allow selecting empty apps ([#723](https://github.com/pyroscope-io/pyroscope/issues/723)) ([2378ab5](https://github.com/pyroscope-io/pyroscope/commit/2378ab5cff3e10c1ad9f4b4edc423f16f975a3e4))



# [0.7.0](https://github.com/pyroscope-io/pyroscope/compare/v0.6.0...v0.7.0) (2022-01-13)


### Bug Fixes

* close response body in traffic-duplicator to fix resource leak. ([#694](https://github.com/pyroscope-io/pyroscope/issues/694)) ([5896982](https://github.com/pyroscope-io/pyroscope/commit/58969821013f15b8f2f425e9d2ffa855f615d8a3))
* pyrobench report path ([#684](https://github.com/pyroscope-io/pyroscope/issues/684)) ([d88bd10](https://github.com/pyroscope-io/pyroscope/commit/d88bd103b18f66ffabb66890a5dc789ba4ede124))


### Features

* add scrape metrics for pull mode ([#678](https://github.com/pyroscope-io/pyroscope/issues/678)) ([0bdb99b](https://github.com/pyroscope-io/pyroscope/commit/0bdb99b2d12eafbedac97565d3579995183e6f84))
* adhoc comparison diff support ([#652](https://github.com/pyroscope-io/pyroscope/issues/652)) ([65b7372](https://github.com/pyroscope-io/pyroscope/commit/65b7372e74540663a44356fd2302a69f55f27e19))
* export standalone html ([#691](https://github.com/pyroscope-io/pyroscope/issues/691)) ([8d20863](https://github.com/pyroscope-io/pyroscope/commit/8d20863e26c9ddc45389b2b37bd7cc9b20881247))
* **frontend:** export pprof format ([#620](https://github.com/pyroscope-io/pyroscope/issues/620)) ([60c305d](https://github.com/pyroscope-io/pyroscope/commit/60c305ddd035a34e319d4b0967e9bb21eb82d5dd))
* **frontend:** new app name selector component ([#682](https://github.com/pyroscope-io/pyroscope/issues/682)) ([b6282c3](https://github.com/pyroscope-io/pyroscope/commit/b6282c34e4985f8ec82c98a9eaa049455487a53b))
* **frontend:** persist sidebar collapsed state ([#699](https://github.com/pyroscope-io/pyroscope/issues/699)) ([c552bc2](https://github.com/pyroscope-io/pyroscope/commit/c552bc2c546960e7351134dfa6d7dbc63e4fb8d0))
* keep existing colors when highlighting ([#714](https://github.com/pyroscope-io/pyroscope/issues/714)) ([ab094c2](https://github.com/pyroscope-io/pyroscope/commit/ab094c258fa3963710dcc137cd9ff046da146be8))


### Performance Improvements

* optimize ingestion flow ([#663](https://github.com/pyroscope-io/pyroscope/issues/663)) ([556a4c6](https://github.com/pyroscope-io/pyroscope/commit/556a4c649e9995058e2fedf7161577eb5991a9fb))
* optimize segment tree serialization ([#695](https://github.com/pyroscope-io/pyroscope/issues/695)) ([091e925](https://github.com/pyroscope-io/pyroscope/commit/091e925ba10dae081059cbc407733413679d3db2))



# [0.6.0](https://github.com/pyroscope-io/pyroscope/compare/v0.5.1...v0.6.0) (2022-01-04)


### Bug Fixes

* **frontend:** add tab panel styles to adhoc comparison component. ([#650](https://github.com/pyroscope-io/pyroscope/issues/650)) ([6537dfe](https://github.com/pyroscope-io/pyroscope/commit/6537dfe8578768db8aa3e31a13a728d7a480d541))
* **frontend:** comparison diff ui fixes ([#627](https://github.com/pyroscope-io/pyroscope/issues/627)) ([202835b](https://github.com/pyroscope-io/pyroscope/commit/202835bcc5f41a5b8858d566b1c1917965a510ff))
* **frontend:** fix flamegraph width in comparison view ([#639](https://github.com/pyroscope-io/pyroscope/issues/639)) ([1e6bef5](https://github.com/pyroscope-io/pyroscope/commit/1e6bef56423b02646f601f19d6c64f7749ef392e))
* **frontend:** fixes golang package name coloring ([#635](https://github.com/pyroscope-io/pyroscope/issues/635)) ([6c390b5](https://github.com/pyroscope-io/pyroscope/commit/6c390b5d27a2ce1d2b61b7875cfaf6d1846937f8))
* **frontend:** keep query param when changing routes ([#674](https://github.com/pyroscope-io/pyroscope/issues/674)) ([389019b](https://github.com/pyroscope-io/pyroscope/commit/389019b8c4127ccd738cce74dbf14deebebcb273))
* **panel:** import @szhsin/react-menu styles in contextmenu ([#669](https://github.com/pyroscope-io/pyroscope/issues/669)) ([2fb0fff](https://github.com/pyroscope-io/pyroscope/commit/2fb0fffd8394c1ec5958a85c0bb2628b946e0ec4))
* register all pprof http handlers ([#672](https://github.com/pyroscope-io/pyroscope/issues/672)) ([f377cf3](https://github.com/pyroscope-io/pyroscope/commit/f377cf3c455cd7231ba1527ada933696fb9c1495))
* try to create data directory if it doesn't exist. ([#646](https://github.com/pyroscope-io/pyroscope/issues/646)) ([eac8c4e](https://github.com/pyroscope-io/pyroscope/commit/eac8c4eead754fe14d92aef14300c3cdbb8cf06f))
* use the correct controller variable. ([#615](https://github.com/pyroscope-io/pyroscope/issues/615)) ([ccd97f9](https://github.com/pyroscope-io/pyroscope/commit/ccd97f9a68a18798c26282e4e74e1bd536b09d87))


### Features

* **frontend:** new tags dropdown ([#642](https://github.com/pyroscope-io/pyroscope/issues/642)) ([6290e45](https://github.com/pyroscope-io/pyroscope/commit/6290e45506193ed95566cb4ab7264b81a32bd266))
* improve datetime format in adhoc output filename. ([f08f498](https://github.com/pyroscope-io/pyroscope/commit/f08f498531c12f8e97007f11d5614a32a433e752))
* **pull-mode:** adds file discovery mechanism ([#662](https://github.com/pyroscope-io/pyroscope/issues/662)) ([35ce0b5](https://github.com/pyroscope-io/pyroscope/commit/35ce0b5071c67e8ae5503b6bd45c9d88a21cbb1f))
* support importing adhoc profiles in pprof or collapsed formats ([#649](https://github.com/pyroscope-io/pyroscope/issues/649)) ([14ee845](https://github.com/pyroscope-io/pyroscope/commit/14ee8457a5a115620660f79bd548bb12a08e10d8))


### Performance Improvements

* adds another direct upstream to improve performance when scraper is overloaded ([#636](https://github.com/pyroscope-io/pyroscope/issues/636)) ([34cfab5](https://github.com/pyroscope-io/pyroscope/commit/34cfab5cdec7d79c0b0822dd3f501a8f46f53b55))
* benchmarking code improvements ([#630](https://github.com/pyroscope-io/pyroscope/issues/630)) ([3aa460c](https://github.com/pyroscope-io/pyroscope/commit/3aa460c826cff7b8494e1d429fe3408d64019244))
* optimize pprof parsing in pull mode. ([#628](https://github.com/pyroscope-io/pyroscope/issues/628)) ([c626be1](https://github.com/pyroscope-io/pyroscope/commit/c626be17f16d9f1132e4f09288dc2495958aff45))



## [0.5.1](https://github.com/pyroscope-io/pyroscope/compare/v0.5.0...v0.5.1) (2021-12-16)


### Bug Fixes

* **frontend:** fixes Timeline component by pinning dependencies to a specific version ([#619](https://github.com/pyroscope-io/pyroscope/issues/619)) ([0324c7c](https://github.com/pyroscope-io/pyroscope/commit/0324c7c202844f21e9c623255a3e476b2e8fd900))


### Features

* **frontend:** allow to export flamegraph json ([#616](https://github.com/pyroscope-io/pyroscope/issues/616)) ([3435a21](https://github.com/pyroscope-io/pyroscope/commit/3435a212aa659d5240f84702926833d351a9d44f))



# [0.5.0](https://github.com/pyroscope-io/pyroscope/compare/v0.4.1...v0.5.0) (2021-12-16)


### Bug Fixes

* assets handling when using base-url argument ([#611](https://github.com/pyroscope-io/pyroscope/issues/611)) ([97a6002](https://github.com/pyroscope-io/pyroscope/commit/97a60023e3e9d5b9fa705eacf0900386ee8367c7))
* avoid converting args to double dash and disable flag parsing for adhoc. ([#609](https://github.com/pyroscope-io/pyroscope/issues/609)) ([f41906d](https://github.com/pyroscope-io/pyroscope/commit/f41906dd23e68de8c861b28b8f40012c6a512901))
* **integrations:** start upstream in clib. ([#602](https://github.com/pyroscope-io/pyroscope/issues/602)) ([0be03c3](https://github.com/pyroscope-io/pyroscope/commit/0be03c3305ffe2723dbbf3a7e59631df09870c07))


### Features

* adhoc comparison UI ([#580](https://github.com/pyroscope-io/pyroscope/issues/580)) ([3272249](https://github.com/pyroscope-io/pyroscope/commit/3272249cfbe308e4b3b2e7ccaa0f446b0ea942e4))
* **frontend:** new sidebar ([#581](https://github.com/pyroscope-io/pyroscope/issues/581)) ([f373706](https://github.com/pyroscope-io/pyroscope/commit/f37370680047055f6d6be97eeb611873a9b44581))



## [0.4.1](https://github.com/pyroscope-io/pyroscope/compare/v0.4.0...v0.4.1) (2021-12-08)


### Bug Fixes

* trigger retention task when no levels are configured ([#588](https://github.com/pyroscope-io/pyroscope/issues/588)) ([141d734](https://github.com/pyroscope-io/pyroscope/commit/141d73406f5482ee14f999ee7721403d3e0d3d99))



# [0.4.0](https://github.com/pyroscope-io/pyroscope/compare/v0.3.1...v0.4.0) (2021-12-08)


### Bug Fixes

* read default config file when it's present ([#585](https://github.com/pyroscope-io/pyroscope/issues/585)) ([7061292](https://github.com/pyroscope-io/pyroscope/commit/7061292ea5cdee9de419e6a99b34f6db2f1eadb2))


### Features

* makes admin command available (GA status) ([8eb788f](https://github.com/pyroscope-io/pyroscope/commit/8eb788f4eb37f08f79d5ba83a6789f22b3dc61f1))



## [0.3.1](https://github.com/pyroscope-io/pyroscope/compare/v0.3.0...v0.3.1) (2021-12-01)


### Bug Fixes

* fixes pprof->trie conversion bug where some samples were dropped ([#575](https://github.com/pyroscope-io/pyroscope/issues/575)) ([cb33851](https://github.com/pyroscope-io/pyroscope/commit/cb33851e55337c0c049d291bf29de8eec860229a))
* generate trie from pprof at scraping correctly ([#577](https://github.com/pyroscope-io/pyroscope/issues/577)) ([bc704f6](https://github.com/pyroscope-io/pyroscope/commit/bc704f6c64fb2e274f7b425b03ea7bd5ef72fe60))
* Prevent byte buffer pool copy. ([#570](https://github.com/pyroscope-io/pyroscope/issues/570)) ([3d1122e](https://github.com/pyroscope-io/pyroscope/commit/3d1122e0ac4a02bc2e78d8e0aac04e34337f4a25))


### Features

* Add adhoc push mode support in clib. ([#576](https://github.com/pyroscope-io/pyroscope/issues/576)) ([b63079f](https://github.com/pyroscope-io/pyroscope/commit/b63079f7f33e0aefde3bf8a9e122e436696bef63))
* support for app deletion with tags ([#569](https://github.com/pyroscope-io/pyroscope/issues/569)) ([7eac3d3](https://github.com/pyroscope-io/pyroscope/commit/7eac3d31620926d6ec5825de6a5551cf57596f50))



# [0.3.0](https://github.com/pyroscope-io/pyroscope/compare/v0.2.5...v0.3.0) (2021-11-29)


### Bug Fixes

* **alpine:** fix the stack overflow on Alpine generated binaries. ([#545](https://github.com/pyroscope-io/pyroscope/issues/545)) ([cd5e4f7](https://github.com/pyroscope-io/pyroscope/commit/cd5e4f7a35450429f7e13634e30fb8272a922caf))
* analytics_test.go on windows ([ad6b20d](https://github.com/pyroscope-io/pyroscope/commit/ad6b20dbe1685b5f56c386fcc2af39b10133d5c3))
* avoid redundant compaction ([#514](https://github.com/pyroscope-io/pyroscope/issues/514)) ([c87f69a](https://github.com/pyroscope-io/pyroscope/commit/c87f69a144d94b469e5d47df11422754c95c6ed2))
* comparison view timeline ([#553](https://github.com/pyroscope-io/pyroscope/issues/553)) ([c615bb7](https://github.com/pyroscope-io/pyroscope/commit/c615bb7340202cf53bd37079a23bb66d40c78dce))
* delete apps functionality ([#551](https://github.com/pyroscope-io/pyroscope/issues/551)) ([09384f3](https://github.com/pyroscope-io/pyroscope/commit/09384f3b186bd7a83ae902a865ed4ff911b0e738))
* **frontend:** fix comparison view ([#549](https://github.com/pyroscope-io/pyroscope/issues/549)) ([4c9d2f8](https://github.com/pyroscope-io/pyroscope/commit/4c9d2f8d4b7a27ee922afa8d7f8c13b6456b7646))
* ignore ErrServerClosed when admin server is closed ([#560](https://github.com/pyroscope-io/pyroscope/issues/560)) ([96ad74f](https://github.com/pyroscope-io/pyroscope/commit/96ad74f6bb3d60d415b7aa1ad5ab8238b1b36407))
* race condition in version update. ([#542](https://github.com/pyroscope-io/pyroscope/issues/542)) ([332a7d5](https://github.com/pyroscope-io/pyroscope/commit/332a7d508aa72453901bfaf654315a79883dca3f))
* timeline guide text alignment ([#565](https://github.com/pyroscope-io/pyroscope/issues/565)) ([efc94a0](https://github.com/pyroscope-io/pyroscope/commit/efc94a0a3fbc07234113df826dc732d51375f59d))
* **windows:** redeclared exec ([#513](https://github.com/pyroscope-io/pyroscope/issues/513)) ([d4ab78d](https://github.com/pyroscope-io/pyroscope/commit/d4ab78d3e370f39acaa8a765294569888cfc41bf))


### Features

* add shell completion for 'admin app delete' cmd ([#535](https://github.com/pyroscope-io/pyroscope/issues/535)) ([f832f53](https://github.com/pyroscope-io/pyroscope/commit/f832f5303ced4b917976abc2e2b5a1dd514cdf02))
* adds adhoc single view ([#546](https://github.com/pyroscope-io/pyroscope/issues/546)) ([4983566](https://github.com/pyroscope-io/pyroscope/commit/4983566e9076223b46e0e0d75a101ad4a89173b2))
* adds support for pprof in /ingest endpoint ([#557](https://github.com/pyroscope-io/pyroscope/issues/557)) ([f233ef3](https://github.com/pyroscope-io/pyroscope/commit/f233ef3c44623e4e0bca143d6d54944fa3723be5))
* initial support for adhoc mode ([#504](https://github.com/pyroscope-io/pyroscope/issues/504)) ([e5d311e](https://github.com/pyroscope-io/pyroscope/commit/e5d311ef90afe8964b046d9be59a4ff38ce7906d))
* pull mode ([#527](https://github.com/pyroscope-io/pyroscope/issues/527)) ([a56aacf](https://github.com/pyroscope-io/pyroscope/commit/a56aacfc0dcbad6bdf6a7478212230f9a2bb4d4e))
* support for pprof and collapsed formats in /render endpoint [#471](https://github.com/pyroscope-io/pyroscope/issues/471) ([#518](https://github.com/pyroscope-io/pyroscope/issues/518)) ([0cbf399](https://github.com/pyroscope-io/pyroscope/commit/0cbf399e441be24520a521732681708b50bf2fde))



## <small>0.2.5 (2021-11-09)</small>

* fix: highlight in diff view (#498) ([fbb826a](https://github.com/pyroscope-io/pyroscope/commit/fbb826a)), closes [#498](https://github.com/pyroscope-io/pyroscope/issues/498)
* fix: linking flags in the linux build system (#499) ([d7446be](https://github.com/pyroscope-io/pyroscope/commit/d7446be)), closes [#499](https://github.com/pyroscope-io/pyroscope/issues/499)
* fix(agent): snapshotting error handling (#510) ([d0e477a](https://github.com/pyroscope-io/pyroscope/commit/d0e477a)), closes [#510](https://github.com/pyroscope-io/pyroscope/issues/510)
* fix(exec): pyspy and rbspy are hanging in blocking mode (#436) ([653a1e3](https://github.com/pyroscope-io/pyroscope/commit/653a1e3)), closes [#436](https://github.com/pyroscope-io/pyroscope/issues/436)
* docs: fix typo in deployment diagram ([297e3da](https://github.com/pyroscope-io/pyroscope/commit/297e3da))
* docs: update deployment diagram for eBPF ([1620650](https://github.com/pyroscope-io/pyroscope/commit/1620650))
* feat: segment level-based retention (#431) ([2a9834f](https://github.com/pyroscope-io/pyroscope/commit/2a9834f)), closes [#431](https://github.com/pyroscope-io/pyroscope/issues/431)
* refactor(config): decouple internal config from public CLI config in exec and server/storage (#508) ([95ceb71](https://github.com/pyroscope-io/pyroscope/commit/95ceb71)), closes [#508](https://github.com/pyroscope-io/pyroscope/issues/508)
* Components lib (#509) ([4b6ca15](https://github.com/pyroscope-io/pyroscope/commit/4b6ca15)), closes [#509](https://github.com/pyroscope-io/pyroscope/issues/509)
* Improve text when there is no data available (#487) ([dcbc39f](https://github.com/pyroscope-io/pyroscope/commit/dcbc39f)), closes [#487](https://github.com/pyroscope-io/pyroscope/issues/487)
* Make sidebar links (#489) ([44c5ac0](https://github.com/pyroscope-io/pyroscope/commit/44c5ac0)), closes [#489](https://github.com/pyroscope-io/pyroscope/issues/489)
* Reduce bundle size (#496) ([4f8ed1b](https://github.com/pyroscope-io/pyroscope/commit/4f8ed1b)), closes [#496](https://github.com/pyroscope-io/pyroscope/issues/496)
* Refactor toolbar/Add "focus on subtree" button (#495) ([b804d57](https://github.com/pyroscope-io/pyroscope/commit/b804d57)), closes [#495](https://github.com/pyroscope-io/pyroscope/issues/495)
* remove unnecessary margin for the buttons (#505) ([2f0efaf](https://github.com/pyroscope-io/pyroscope/commit/2f0efaf)), closes [#505](https://github.com/pyroscope-io/pyroscope/issues/505)
* chore: get rid of pdf export (#493) ([6879d45](https://github.com/pyroscope-io/pyroscope/commit/6879d45)), closes [#493](https://github.com/pyroscope-io/pyroscope/issues/493)



## <small>0.2.4 (2021-11-01)</small>

* Fix sidebar getting out of sync (#488) ([29c0153](https://github.com/pyroscope-io/pyroscope/commit/29c0153)), closes [#488](https://github.com/pyroscope-io/pyroscope/issues/488)
* fix: fix highlight and its test (#491) ([dd6d6b5](https://github.com/pyroscope-io/pyroscope/commit/dd6d6b5)), closes [#491](https://github.com/pyroscope-io/pyroscope/issues/491)
* perf: improve flamegraph by memoizing table (#484) ([76cd773](https://github.com/pyroscope-io/pyroscope/commit/76cd773)), closes [#484](https://github.com/pyroscope-io/pyroscope/issues/484)



## <small>0.2.3 (2021-11-01)</small>

* feat: improves ebpf integration (#490) ([24f5872](https://github.com/pyroscope-io/pyroscope/commit/24f5872)), closes [#490](https://github.com/pyroscope-io/pyroscope/issues/490)
* Feature/441 "focus on this subtree" (#480) ([cfe0ae8](https://github.com/pyroscope-io/pyroscope/commit/cfe0ae8)), closes [#480](https://github.com/pyroscope-io/pyroscope/issues/480)
* Flamegraph refactor (#477) ([deaed44](https://github.com/pyroscope-io/pyroscope/commit/deaed44)), closes [#477](https://github.com/pyroscope-io/pyroscope/issues/477)
* docs: Add Django Example to Python Examples Folder (#472) ([0dab599](https://github.com/pyroscope-io/pyroscope/commit/0dab599)), closes [#472](https://github.com/pyroscope-io/pyroscope/issues/472)
* docs: add java to agent diagram ([8653517](https://github.com/pyroscope-io/pyroscope/commit/8653517))
* docs: adds license scan badge ([5dbd5e7](https://github.com/pyroscope-io/pyroscope/commit/5dbd5e7))
* docs: delete extra blog post doc ([6d807ae](https://github.com/pyroscope-io/pyroscope/commit/6d807ae))
* docs: update ruby example images (#486) ([38554ac](https://github.com/pyroscope-io/pyroscope/commit/38554ac)), closes [#486](https://github.com/pyroscope-io/pyroscope/issues/486)
* docs: update supported languages ([4cd06a2](https://github.com/pyroscope-io/pyroscope/commit/4cd06a2))
* build: optimizes dockerfile, cuts docker image size in half (#476) ([ad94706](https://github.com/pyroscope-io/pyroscope/commit/ad94706)), closes [#476](https://github.com/pyroscope-io/pyroscope/issues/476)
* build(frontend): disables watch mode, improves error message (#482) ([67dc7f1](https://github.com/pyroscope-io/pyroscope/commit/67dc7f1)), closes [#482](https://github.com/pyroscope-io/pyroscope/issues/482)
* ci: adopts conventional commit convention for changelog update commits ([8bccd3d](https://github.com/pyroscope-io/pyroscope/commit/8bccd3d))



## <small>0.2.2 (2021-10-19)</small>

* feat(golang): support for golang profiling labels (#470) ([3286f05](https://github.com/pyroscope-io/pyroscope/commit/3286f05)), closes [#470](https://github.com/pyroscope-io/pyroscope/issues/470)
* feat(ops): implement out of space notification #348 (#456) ([e33e0b3](https://github.com/pyroscope-io/pyroscope/commit/e33e0b3)), closes [#348](https://github.com/pyroscope-io/pyroscope/issues/348) [#456](https://github.com/pyroscope-io/pyroscope/issues/456)
* refactor(flamegraph): abstracts canvas out of flamegraph renderer (#466) ([8b7bf9d](https://github.com/pyroscope-io/pyroscope/commit/8b7bf9d)), closes [#466](https://github.com/pyroscope-io/pyroscope/issues/466)



## <small>0.2.1 (2021-10-18)</small>

* Cleanup (#439) ([29781f8](https://github.com/pyroscope-io/pyroscope/commit/29781f8)), closes [#439](https://github.com/pyroscope-io/pyroscope/issues/439)
* cleanup / tooltip issues (#452) ([9486ee0](https://github.com/pyroscope-io/pyroscope/commit/9486ee0)), closes [#452](https://github.com/pyroscope-io/pyroscope/issues/452)
* fix: handle a BadgerDB panic related to incorrectly set permissions (#464) ([840cf03](https://github.com/pyroscope-io/pyroscope/commit/840cf03)), closes [#464](https://github.com/pyroscope-io/pyroscope/issues/464)
* fix(dotnet): enable dotnetspy for macos build (#469) ([c155ddc](https://github.com/pyroscope-io/pyroscope/commit/c155ddc)), closes [#469](https://github.com/pyroscope-io/pyroscope/issues/469)
* fix(python): updates py-spy version (#440) ([1d5868b](https://github.com/pyroscope-io/pyroscope/commit/1d5868b)), closes [#440](https://github.com/pyroscope-io/pyroscope/issues/440) [#428](https://github.com/pyroscope-io/pyroscope/issues/428)
* ci: adds basic e2e tests (#446) ([2fd51d1](https://github.com/pyroscope-io/pyroscope/commit/2fd51d1)), closes [#446](https://github.com/pyroscope-io/pyroscope/issues/446)
* ci: generate random app name in Cypress test (#463) ([bc427f2](https://github.com/pyroscope-io/pyroscope/commit/bc427f2)), closes [#463](https://github.com/pyroscope-io/pyroscope/issues/463)
* chore: add codecov for js tests (#467) ([a673aea](https://github.com/pyroscope-io/pyroscope/commit/a673aea)), closes [#467](https://github.com/pyroscope-io/pyroscope/issues/467)
* chore: add size-limit action ([56c519d](https://github.com/pyroscope-io/pyroscope/commit/56c519d))
* chore: add yarn build command ([02e3b1e](https://github.com/pyroscope-io/pyroscope/commit/02e3b1e))
* chore: FlameQL code refactoring (#453) ([ec5b5fa](https://github.com/pyroscope-io/pyroscope/commit/ec5b5fa)), closes [#453](https://github.com/pyroscope-io/pyroscope/issues/453)
* chore: refactor metrics exporter (#444) ([8fd0d63](https://github.com/pyroscope-io/pyroscope/commit/8fd0d63)), closes [#444](https://github.com/pyroscope-io/pyroscope/issues/444)
* chore: separate size-limit into its own workflow (#465) ([224ac5c](https://github.com/pyroscope-io/pyroscope/commit/224ac5c)), closes [#465](https://github.com/pyroscope-io/pyroscope/issues/465)
* chore(analytics): adds java to list of integrations we track (#450) ([65358d9](https://github.com/pyroscope-io/pyroscope/commit/65358d9)), closes [#450](https://github.com/pyroscope-io/pyroscope/issues/450)
* feat: add basic context menu (#460) ([3df5d9d](https://github.com/pyroscope-io/pyroscope/commit/3df5d9d)), closes [#460](https://github.com/pyroscope-io/pyroscope/issues/460)
* feat(profiler): add support for dynamic tags (#437) ([4ab01ce](https://github.com/pyroscope-io/pyroscope/commit/4ab01ce)), closes [#437](https://github.com/pyroscope-io/pyroscope/issues/437)
* docs: add chinese translation for Python docs (#455) ([4e01c0a](https://github.com/pyroscope-io/pyroscope/commit/4e01c0a)), closes [#455](https://github.com/pyroscope-io/pyroscope/issues/455)
* docs: add Chinese translation for Ruby docs (#457) ([d1ab6da](https://github.com/pyroscope-io/pyroscope/commit/d1ab6da)), closes [#457](https://github.com/pyroscope-io/pyroscope/issues/457)
* docs: Add FastAPI Example to Python Examples Folder (#443) ([1292277](https://github.com/pyroscope-io/pyroscope/commit/1292277)), closes [#443](https://github.com/pyroscope-io/pyroscope/issues/443)
* dos: Add FastAPI Example to Python Examples Folder (#443) ([720c935](https://github.com/pyroscope-io/pyroscope/commit/720c935)), closes [#443](https://github.com/pyroscope-io/pyroscope/issues/443)



## 0.2.0 (2021-10-05)

* docs: Cleanup docs for Pip package and Ruby gem (#438) ([314dc4b](https://github.com/pyroscope-io/pyroscope/commit/314dc4b)), closes [#438](https://github.com/pyroscope-io/pyroscope/issues/438)
* docs: update Pyroscope example with pip package (#434) ([c705f38](https://github.com/pyroscope-io/pyroscope/commit/c705f38)), closes [#434](https://github.com/pyroscope-io/pyroscope/issues/434)
* docs: Update Ruby Example with new Ruby gem (#429) ([50b3e38](https://github.com/pyroscope-io/pyroscope/commit/50b3e38)), closes [#429](https://github.com/pyroscope-io/pyroscope/issues/429)
* 408 switch from samples to percentages for diff view (#432) ([a175714](https://github.com/pyroscope-io/pyroscope/commit/a175714)), closes [#432](https://github.com/pyroscope-io/pyroscope/issues/432)
* updates python example ([f461d0c](https://github.com/pyroscope-io/pyroscope/commit/f461d0c))
* chore: fix flaky query test (#430) ([8dbfe94](https://github.com/pyroscope-io/pyroscope/commit/8dbfe94)), closes [#430](https://github.com/pyroscope-io/pyroscope/issues/430)
* chore(webapp): implement live reload (#435) ([8f06505](https://github.com/pyroscope-io/pyroscope/commit/8f06505)), closes [#435](https://github.com/pyroscope-io/pyroscope/issues/435)
* ci: make go lint fail build (#427) ([bac1171](https://github.com/pyroscope-io/pyroscope/commit/bac1171)), closes [#427](https://github.com/pyroscope-io/pyroscope/issues/427) [#430](https://github.com/pyroscope-io/pyroscope/issues/430)



## 0.1.0 (2021-09-30)

* docs: update deployment image ([59c459c](https://github.com/pyroscope-io/pyroscope/commit/59c459c))
* docs: update readme (#426) ([1dd94cb](https://github.com/pyroscope-io/pyroscope/commit/1dd94cb)), closes [#426](https://github.com/pyroscope-io/pyroscope/issues/426)
* docs: update the translation to keep up with the original (#423) ([544f49e](https://github.com/pyroscope-io/pyroscope/commit/544f49e)), closes [#423](https://github.com/pyroscope-io/pyroscope/issues/423)
* docs: updates ruby / python example (#422) ([0dd5756](https://github.com/pyroscope-io/pyroscope/commit/0dd5756)), closes [#422](https://github.com/pyroscope-io/pyroscope/issues/422)
* bug: fix issues with tag intersections in query engine (#425) ([bf34937](https://github.com/pyroscope-io/pyroscope/commit/bf34937)), closes [#425](https://github.com/pyroscope-io/pyroscope/issues/425)
* add basic visual testing test (#419) ([e3e170d](https://github.com/pyroscope-io/pyroscope/commit/e3e170d)), closes [#419](https://github.com/pyroscope-io/pyroscope/issues/419)
* Visual testing (#421) ([af91c14](https://github.com/pyroscope-io/pyroscope/commit/af91c14)), closes [#421](https://github.com/pyroscope-io/pyroscope/issues/421)
* feat: TLS support for the server #185 (#404) ([6eb4a6c](https://github.com/pyroscope-io/pyroscope/commit/6eb4a6c)), closes [#185](https://github.com/pyroscope-io/pyroscope/issues/185) [#404](https://github.com/pyroscope-io/pyroscope/issues/404)



## <small>0.0.41 (2021-09-24)</small>

* fixes mac builds ([5f4e0e3](https://github.com/pyroscope-io/pyroscope/commit/5f4e0e3))



## <small>0.0.40 (2021-09-24)</small>

* Add placeholder for agent / server communication ([de9e4f8](https://github.com/pyroscope-io/pyroscope/commit/de9e4f8))
* adds a java example (#410) ([1a4741c](https://github.com/pyroscope-io/pyroscope/commit/1a4741c)), closes [#410](https://github.com/pyroscope-io/pyroscope/issues/410)
* Adds gem and pip examples to the repo (#420) ([7a74083](https://github.com/pyroscope-io/pyroscope/commit/7a74083)), closes [#420](https://github.com/pyroscope-io/pyroscope/issues/420)
* basic pyrobench cli reusing config/config.go ([8841206](https://github.com/pyroscope-io/pyroscope/commit/8841206))
* basic pyrobench cli using its own config.go ([a0659df](https://github.com/pyroscope-io/pyroscope/commit/a0659df))
* better naming ([7a466e6](https://github.com/pyroscope-io/pyroscope/commit/7a466e6))
* Bump go version to 1.17.0 (#372) ([e650f3e](https://github.com/pyroscope-io/pyroscope/commit/e650f3e)), closes [#372](https://github.com/pyroscope-io/pyroscope/issues/372)
* Config fixes, test cases (#376) ([6b7911e](https://github.com/pyroscope-io/pyroscope/commit/6b7911e)), closes [#376](https://github.com/pyroscope-io/pyroscope/issues/376)
* Enhancement/split flamegraphrenderer (#360) ([230699f](https://github.com/pyroscope-io/pyroscope/commit/230699f)), closes [#360](https://github.com/pyroscope-io/pyroscope/issues/360) [#382](https://github.com/pyroscope-io/pyroscope/issues/382)
* fix env var prefix ([6ac82b9](https://github.com/pyroscope-io/pyroscope/commit/6ac82b9))
* fix race conditions that were crashing cypress tests (#389) ([86778e5](https://github.com/pyroscope-io/pyroscope/commit/86778e5)), closes [#389](https://github.com/pyroscope-io/pyroscope/issues/389)
* Fix reset view (#414) ([f8da10f](https://github.com/pyroscope-io/pyroscope/commit/f8da10f)), closes [#414](https://github.com/pyroscope-io/pyroscope/issues/414)
* Fix search bar (#413) ([0b17e11](https://github.com/pyroscope-io/pyroscope/commit/0b17e11)), closes [#413](https://github.com/pyroscope-io/pyroscope/issues/413)
* fix search bar (#415) ([8f86341](https://github.com/pyroscope-io/pyroscope/commit/8f86341)), closes [#415](https://github.com/pyroscope-io/pyroscope/issues/415)
* Fixes diff view color issue (#383) ([897de97](https://github.com/pyroscope-io/pyroscope/commit/897de97)), closes [#383](https://github.com/pyroscope-io/pyroscope/issues/383)
* generate a random prefix for server bench pr docker-compose (#392) ([8993204](https://github.com/pyroscope-io/pyroscope/commit/8993204)), closes [#392](https://github.com/pyroscope-io/pyroscope/issues/392)
* makes it so that sign out button is hidden when there's no auth methods set up (#377) ([ce5fb76](https://github.com/pyroscope-io/pyroscope/commit/ce5fb76)), closes [#377](https://github.com/pyroscope-io/pyroscope/issues/377)
* move cmd/logging stuff to /pkg/cli for better reuse ([9f28783](https://github.com/pyroscope-io/pyroscope/commit/9f28783))
* move command.go stuff to /pkg ([f8a6f57](https://github.com/pyroscope-io/pyroscope/commit/f8a6f57))
* move gradient banner generation to pkg/cli ([1b82733](https://github.com/pyroscope-io/pyroscope/commit/1b82733))
* move usage.go to pkg/cli for better reuse ([2d2007e](https://github.com/pyroscope-io/pyroscope/commit/2d2007e))
* Optimize cache persistence (#385) ([106ebde](https://github.com/pyroscope-io/pyroscope/commit/106ebde)), closes [#385](https://github.com/pyroscope-io/pyroscope/issues/385)
* Optimize ingestion allocations (#411) ([ec880c5](https://github.com/pyroscope-io/pyroscope/commit/ec880c5)), closes [#411](https://github.com/pyroscope-io/pyroscope/issues/411)
* serve assets gzipped #342 (#381) ([0039ec5](https://github.com/pyroscope-io/pyroscope/commit/0039ec5)), closes [#342](https://github.com/pyroscope-io/pyroscope/issues/342) [#381](https://github.com/pyroscope-io/pyroscope/issues/381) [#342](https://github.com/pyroscope-io/pyroscope/issues/342)
* Server PR benchmark (#373) ([0403cee](https://github.com/pyroscope-io/pyroscope/commit/0403cee)), closes [#373](https://github.com/pyroscope-io/pyroscope/issues/373)
* support taking screenshot of dashboard with rows (#395) ([c2d922f](https://github.com/pyroscope-io/pyroscope/commit/c2d922f)), closes [#395](https://github.com/pyroscope-io/pyroscope/issues/395)
* tests for app change dropdown (issue #356) (#386) ([2332075](https://github.com/pyroscope-io/pyroscope/commit/2332075)), closes [#356](https://github.com/pyroscope-io/pyroscope/issues/356) [#386](https://github.com/pyroscope-io/pyroscope/issues/386)
* tests for table sorting (issue #356) (#407) ([9bdabca](https://github.com/pyroscope-io/pyroscope/commit/9bdabca)), closes [#356](https://github.com/pyroscope-io/pyroscope/issues/356) [#407](https://github.com/pyroscope-io/pyroscope/issues/407)
* tests for table/both/flamegraph buttons (issue #356) (#375) ([75b0357](https://github.com/pyroscope-io/pyroscope/commit/75b0357)), closes [#356](https://github.com/pyroscope-io/pyroscope/issues/356) [#375](https://github.com/pyroscope-io/pyroscope/issues/375)
* Updates README with info about Java integration ([a6e010b](https://github.com/pyroscope-io/pyroscope/commit/a6e010b))
* frontend: apply prettier to cypress/webapp (#417) ([fffac60](https://github.com/pyroscope-io/pyroscope/commit/fffac60)), closes [#417](https://github.com/pyroscope-io/pyroscope/issues/417)
* benchmark: add cmd to generate a meta report (#396) ([239b28b](https://github.com/pyroscope-io/pyroscope/commit/239b28b)), closes [#396](https://github.com/pyroscope-io/pyroscope/issues/396)
* cypress: cleanup nasty waitInDevMode (#380) ([c831911](https://github.com/pyroscope-io/pyroscope/commit/c831911)), closes [#380](https://github.com/pyroscope-io/pyroscope/issues/380)
* pyrobench: basic load generation ([1536b75](https://github.com/pyroscope-io/pyroscope/commit/1536b75))
* pyrobench: copy over refactor from main ([387170c](https://github.com/pyroscope-io/pyroscope/commit/387170c))
* pyrobench: make cli style same as the original cli ([580ed5f](https://github.com/pyroscope-io/pyroscope/commit/580ed5f))
* pyrobench: use a wrapper config ([e0e614c](https://github.com/pyroscope-io/pyroscope/commit/e0e614c))



## <small>0.0.39 (2021-09-01)</small>

* Add tests for exec command ([70164d6](https://github.com/pyroscope-io/pyroscope/commit/70164d6))
* Fix argument processing ([50351b1](https://github.com/pyroscope-io/pyroscope/commit/50351b1))
* reduces bundle size (#362) ([90cfee2](https://github.com/pyroscope-io/pyroscope/commit/90cfee2)), closes [#362](https://github.com/pyroscope-io/pyroscope/issues/362)
* Refactor CLI (#363) ([8d46308](https://github.com/pyroscope-io/pyroscope/commit/8d46308)), closes [#363](https://github.com/pyroscope-io/pyroscope/issues/363)



## <small>0.0.38 (2021-08-31)</small>

* "native" integrations for ruby, python and php (#274) ([9df0684](https://github.com/pyroscope-io/pyroscope/commit/9df0684)), closes [#274](https://github.com/pyroscope-io/pyroscope/issues/274) [#352](https://github.com/pyroscope-io/pyroscope/issues/352)
* Add a POST handler for `render-diff` (#338) ([f3186f4](https://github.com/pyroscope-io/pyroscope/commit/f3186f4)), closes [#338](https://github.com/pyroscope-io/pyroscope/issues/338)
* Add cypress (#359) ([7c6224c](https://github.com/pyroscope-io/pyroscope/commit/7c6224c)), closes [#359](https://github.com/pyroscope-io/pyroscope/issues/359)
* Add dbtool ([1dc036f](https://github.com/pyroscope-io/pyroscope/commit/1dc036f))
* added a make help target for listing out all the commands ([12f8317](https://github.com/pyroscope-io/pyroscope/commit/12f8317))
* adds --ignore-engines to yarn install ([44ff964](https://github.com/pyroscope-io/pyroscope/commit/44ff964))
* adds .devcontainer directory for github codespaces ([caf05f2](https://github.com/pyroscope-io/pyroscope/commit/caf05f2))
* Adds an "Updates Available" label to the footer (#333) ([4923be0](https://github.com/pyroscope-io/pyroscope/commit/4923be0)), closes [#333](https://github.com/pyroscope-io/pyroscope/issues/333)
* Adds frontend support for notifications (#334) ([bdaaaf6](https://github.com/pyroscope-io/pyroscope/commit/bdaaaf6)), closes [#334](https://github.com/pyroscope-io/pyroscope/issues/334)
* adds github workflows for checking the file sizes and js bundle sizes (#341) ([f4ada2a](https://github.com/pyroscope-io/pyroscope/commit/f4ada2a)), closes [#341](https://github.com/pyroscope-io/pyroscope/issues/341)
* Applying linting rules (#354) ([785c839](https://github.com/pyroscope-io/pyroscope/commit/785c839)), closes [#354](https://github.com/pyroscope-io/pyroscope/issues/354)
* basic layout and API requests handlers ([3699739](https://github.com/pyroscope-io/pyroscope/commit/3699739))
* Benchmark dashboard (#349) ([1e1267a](https://github.com/pyroscope-io/pyroscope/commit/1e1267a)), closes [#349](https://github.com/pyroscope-io/pyroscope/issues/349)
* benchmark dockerfile fix ([a24e358](https://github.com/pyroscope-io/pyroscope/commit/a24e358))
* benchmark improvements ([7ef543f](https://github.com/pyroscope-io/pyroscope/commit/7ef543f))
* Change default diff color to grey ([615e268](https://github.com/pyroscope-io/pyroscope/commit/615e268))
* cleans up frontend part of tags ([aef7828](https://github.com/pyroscope-io/pyroscope/commit/aef7828))
* code highlight ([6a0fa85](https://github.com/pyroscope-io/pyroscope/commit/6a0fa85))
* default name fix ([000b348](https://github.com/pyroscope-io/pyroscope/commit/000b348))
* Feature/grafana example (#331) ([bc34086](https://github.com/pyroscope-io/pyroscope/commit/bc34086)), closes [#331](https://github.com/pyroscope-io/pyroscope/issues/331)
* Fit mode (#353) ([508bdfc](https://github.com/pyroscope-io/pyroscope/commit/508bdfc)), closes [#353](https://github.com/pyroscope-io/pyroscope/issues/353) [#358](https://github.com/pyroscope-io/pyroscope/issues/358)
* fix bad setstate and add safe replace ([61ea327](https://github.com/pyroscope-io/pyroscope/commit/61ea327))
* Fix CPU check ([a005bc2](https://github.com/pyroscope-io/pyroscope/commit/a005bc2))
* fix diff tree ([d695d4e](https://github.com/pyroscope-io/pyroscope/commit/d695d4e))
* fix lint error ([00d2a64](https://github.com/pyroscope-io/pyroscope/commit/00d2a64))
* fix timeframe reset ([98721fa](https://github.com/pyroscope-io/pyroscope/commit/98721fa))
* fixes a bug where app name changes didn't trigger a render ([153b95d](https://github.com/pyroscope-io/pyroscope/commit/153b95d))
* fixes build issues ([3152382](https://github.com/pyroscope-io/pyroscope/commit/3152382))
* fixes mac arm64 build ([45b7079](https://github.com/pyroscope-io/pyroscope/commit/45b7079))
* Http and info metrics (#347) ([7a7d2ea](https://github.com/pyroscope-io/pyroscope/commit/7a7d2ea)), closes [#347](https://github.com/pyroscope-io/pyroscope/issues/347)
* implement diff rendering (#289) ([e089d5b](https://github.com/pyroscope-io/pyroscope/commit/e089d5b)), closes [#289](https://github.com/pyroscope-io/pyroscope/issues/289)
* improve tree diff with prepend ([899d178](https://github.com/pyroscope-io/pyroscope/commit/899d178))
* Improvement/dpr style (#315) ([313a320](https://github.com/pyroscope-io/pyroscope/commit/313a320)), closes [#315](https://github.com/pyroscope-io/pyroscope/issues/315)
* improves make server command, allows for env override of arguments ([7d6f7a5](https://github.com/pyroscope-io/pyroscope/commit/7d6f7a5))
* js size limit github action adjustments ([28e3e9c](https://github.com/pyroscope-io/pyroscope/commit/28e3e9c))
* js test fixes ([fda1a9a](https://github.com/pyroscope-io/pyroscope/commit/fda1a9a))
* linted js code ([999aabb](https://github.com/pyroscope-io/pyroscope/commit/999aabb))
* loading state and selected state indication ([402f911](https://github.com/pyroscope-io/pyroscope/commit/402f911))
* Metrics rename (#346) ([c22739d](https://github.com/pyroscope-io/pyroscope/commit/c22739d)), closes [#346](https://github.com/pyroscope-io/pyroscope/issues/346) [/github.com/prometheus/client_golang/issues/716#issuecomment-590282553](https://github.com//github.com/prometheus/client_golang/issues/716/issues/issuecomment-590282553)
* Migration to Cobra/Viper (#300) ([df05add](https://github.com/pyroscope-io/pyroscope/commit/df05add)), closes [#300](https://github.com/pyroscope-io/pyroscope/issues/300)
* multiple tags and request with query ([c52bbae](https://github.com/pyroscope-io/pyroscope/commit/c52bbae))
* Refactor prometheus metrics (#335) ([673ae2b](https://github.com/pyroscope-io/pyroscope/commit/673ae2b)), closes [#335](https://github.com/pyroscope-io/pyroscope/issues/335) [/github.com/prometheus/client_golang/issues/716#issuecomment-590282553](https://github.com//github.com/prometheus/client_golang/issues/716/issues/issuecomment-590282553)
* removes js size limit action as it's too noisy ([f3ac62e](https://github.com/pyroscope-io/pyroscope/commit/f3ac62e))
* restore tags from url params ([9795d17](https://github.com/pyroscope-io/pyroscope/commit/9795d17))
* Revert "improve tree.Insert" ([5a09a2f](https://github.com/pyroscope-io/pyroscope/commit/5a09a2f))
* size report improvements ([3d77938](https://github.com/pyroscope-io/pyroscope/commit/3d77938))
* small readme fix ([de87046](https://github.com/pyroscope-io/pyroscope/commit/de87046))
* support for query in label endpoints ([3865f98](https://github.com/pyroscope-io/pyroscope/commit/3865f98))
* switch pkger to goembed (#340) ([c566451](https://github.com/pyroscope-io/pyroscope/commit/c566451)), closes [#340](https://github.com/pyroscope-io/pyroscope/issues/340)
* testing js size limit github action ([3685d90](https://github.com/pyroscope-io/pyroscope/commit/3685d90))
* update scripts/decode-resp for format=double ([1e8c9e5](https://github.com/pyroscope-io/pyroscope/commit/1e8c9e5))
* dashboard: initial version created with jsonnet (#339) ([0d31cdf](https://github.com/pyroscope-io/pyroscope/commit/0d31cdf)), closes [#339](https://github.com/pyroscope-io/pyroscope/issues/339)



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

