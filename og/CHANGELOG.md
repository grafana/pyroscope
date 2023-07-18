## [0.37.2](https://github.com/pyroscope-io/pyroscope/compare/v0.37.1...v0.37.2) (2023-02-15)


### Bug Fixes

* **jfr:** do no try to decompress labels if there are no labels ([#1852](https://github.com/pyroscope-io/pyroscope/issues/1852)) ([65e1d69](https://github.com/pyroscope-io/pyroscope/commit/65e1d6923461cb29ca46fe777bd6355bbaba1d06))
* **pprof parsing:** initialize function to zero values ([#1837](https://github.com/pyroscope-io/pyroscope/issues/1837)) ([692f11b](https://github.com/pyroscope-io/pyroscope/commit/692f11bf68c9d08ccb219bf79bfa956fd13281c9))


### Features

* **jfr:** live objects ([#1849](https://github.com/pyroscope-io/pyroscope/issues/1849)) ([001e1e1](https://github.com/pyroscope-io/pyroscope/commit/001e1e195d00912704fde9bfbf72b5544c264015))


### Performance Improvements

* **flamegraph:** don't convert to graphviz format unnecessarily ([#1834](https://github.com/pyroscope-io/pyroscope/issues/1834)) ([8f78e54](https://github.com/pyroscope-io/pyroscope/commit/8f78e54d75cf6f8067ef38da5b2c2eb15860ec09))



## [0.37.1](https://github.com/pyroscope-io/pyroscope/compare/v0.37.0...v0.37.1) (2023-01-31)


### Bug Fixes

* **pprof parsing:** decrease number of allocations during stack hash ([#1822](https://github.com/pyroscope-io/pyroscope/issues/1822)) ([f474c2d](https://github.com/pyroscope-io/pyroscope/commit/f474c2dbc7fed1dad9a2392257ae6fc80fdb4010))
* self profiling sample type config ([#1827](https://github.com/pyroscope-io/pyroscope/issues/1827)) ([f78fdc0](https://github.com/pyroscope-io/pyroscope/commit/f78fdc0867100259b49b790c6e5ee64ac887f8df))


### Features

* pprof write batch parsing ([#1816](https://github.com/pyroscope-io/pyroscope/issues/1816)) ([32593be](https://github.com/pyroscope-io/pyroscope/commit/32593be8efc5b8457889ab78a5bc3d8aa2eb1a04))



# [0.37.0](https://github.com/pyroscope-io/pyroscope/compare/v0.36.0...v0.37.0) (2023-01-24)


### Bug Fixes

* **webapp:** make API table header match the actual content ([#1802](https://github.com/pyroscope-io/pyroscope/issues/1802)) ([3aac1df](https://github.com/pyroscope-io/pyroscope/commit/3aac1df0bb3dfdec0b38067923f2a4624e1eb11c))


### Features

* graphviz visualization support ([#1759](https://github.com/pyroscope-io/pyroscope/issues/1759)) ([ca855d2](https://github.com/pyroscope-io/pyroscope/commit/ca855d2eb424590393d8c0086a1ffcd00f2bc88c))
* pprof streaming parsing ([#1799](https://github.com/pyroscope-io/pyroscope/issues/1799)) ([7ea85f7](https://github.com/pyroscope-io/pyroscope/commit/7ea85f74c8be0dbf288c20f1e0c08967d1a36ad9))
* **pprof:** parsing arenas ([#1804](https://github.com/pyroscope-io/pyroscope/issues/1804)) ([4bc7fca](https://github.com/pyroscope-io/pyroscope/commit/4bc7fca364feb998cdb638ca4d3c326050b41f32))
* **webapp:** add annotations rendering to all timelines ([#1807](https://github.com/pyroscope-io/pyroscope/issues/1807)) ([6144df4](https://github.com/pyroscope-io/pyroscope/commit/6144df404e649c3bdf8460baf8d6da251752c074))
* **webapp:** sync crosshair in different timelines ([#1813](https://github.com/pyroscope-io/pyroscope/issues/1813)) ([e8f14bd](https://github.com/pyroscope-io/pyroscope/commit/e8f14bd79df15b099c4d45589ea22ed4e7297f95))


### Reverts

* Revert "feat: cumulative pprof merge for pull mode (#1794)" (#1811) ([086d3b2](https://github.com/pyroscope-io/pyroscope/commit/086d3b2ac8d3be26d7c36188221560a2ae7bd1ce)), closes [#1794](https://github.com/pyroscope-io/pyroscope/issues/1794) [#1811](https://github.com/pyroscope-io/pyroscope/issues/1811)



# [0.36.0](https://github.com/pyroscope-io/pyroscope/compare/v0.35.1...v0.36.0) (2022-12-16)


### Bug Fixes

* build packages/pyroscope-datasource-plugin/docker-compose.yml ([76cf195](https://github.com/pyroscope-io/pyroscope/commit/76cf1959029973ea6be57cb8026f613ef60d52de))
* build packages/pyroscope-datasource-plugin/docker-compose.yml ([0168ae2](https://github.com/pyroscope-io/pyroscope/commit/0168ae22ea24f452a1b7dfb0a8e9dc649aa7312d))
* **flamegraph:** increase specificity of flamegraph tooltip table styling ([#1778](https://github.com/pyroscope-io/pyroscope/issues/1778)) ([6648fc5](https://github.com/pyroscope-io/pyroscope/commit/6648fc59b1da14aa7146a0b5dc3f72c29733c63e))
* **webapp:** timeline ticks overlapping ([#1786](https://github.com/pyroscope-io/pyroscope/issues/1786)) ([1a6b52d](https://github.com/pyroscope-io/pyroscope/commit/1a6b52d98dac60272ff5753542b7e9a598546dbd))
* **webapp:** toolbar overlaps annotation ([#1785](https://github.com/pyroscope-io/pyroscope/issues/1785)) ([24722d2](https://github.com/pyroscope-io/pyroscope/commit/24722d2843539deef587781612d3ccee89e2e9d5))


### Features

* cumulative pprof merge for pull mode ([#1794](https://github.com/pyroscope-io/pyroscope/issues/1794)) ([89265cd](https://github.com/pyroscope-io/pyroscope/commit/89265cd7f6b1d6fe57fc10b67a1b3c97ef4aa635))



## [0.35.1](https://github.com/pyroscope-io/pyroscope/compare/v0.35.0...v0.35.1) (2022-12-01)


### Bug Fixes

* **flamegraph:** Make table tooltip invisible when user not hovering on table ([#1749](https://github.com/pyroscope-io/pyroscope/issues/1749)) ([5210aa7](https://github.com/pyroscope-io/pyroscope/commit/5210aa72724f7ac2f2c639fe6584d565d7ae1b75))
* small toolbar fixes ([#1777](https://github.com/pyroscope-io/pyroscope/issues/1777)) ([196c6d8](https://github.com/pyroscope-io/pyroscope/commit/196c6d84adeb91efb4a8e4ee9606b5bdcb6a31f4))
* tags loading ([#1784](https://github.com/pyroscope-io/pyroscope/issues/1784)) ([fb4fee8](https://github.com/pyroscope-io/pyroscope/commit/fb4fee858d749c61362c5a95c20fc0213d1e3a7c))
* update tag explorer dropdown ([#1772](https://github.com/pyroscope-io/pyroscope/issues/1772)) ([e04abd7](https://github.com/pyroscope-io/pyroscope/commit/e04abd7704861ac4bc0b7d0d9a420c37ab628a7a))


### Features

* fix toolbar on narrow screens ([#1754](https://github.com/pyroscope-io/pyroscope/issues/1754)) ([78b27d8](https://github.com/pyroscope-io/pyroscope/commit/78b27d8cc8a93149c79f6d4103e8ae81d7b3b024))
* **flamegraph:** allow sorting by diff percentage ([#1776](https://github.com/pyroscope-io/pyroscope/issues/1776)) ([8d9c838](https://github.com/pyroscope-io/pyroscope/commit/8d9c838d7b96daf1e1aa482b1f2d0820bd81f8ed))



# [0.35.0](https://github.com/pyroscope-io/pyroscope/compare/v0.34.1...v0.35.0) (2022-11-28)


### Bug Fixes

* **flamegraph:** fix dropdown menu opening ([#1755](https://github.com/pyroscope-io/pyroscope/issues/1755)) ([0b1acef](https://github.com/pyroscope-io/pyroscope/commit/0b1acef9c5d5a2a268f952a529d12f784d257842))
* make table rows only take one line ([#1765](https://github.com/pyroscope-io/pyroscope/issues/1765)) ([6d33d42](https://github.com/pyroscope-io/pyroscope/commit/6d33d426b4f94833059fea5ca995bb5aa9dd022a))
* move sandwich view message to correct location ([#1767](https://github.com/pyroscope-io/pyroscope/issues/1767)) ([6720c3f](https://github.com/pyroscope-io/pyroscope/commit/6720c3f2de303b25f2031d8c09bbb330561ddde4))
* tag explorer loading spinner ([#1748](https://github.com/pyroscope-io/pyroscope/issues/1748)) ([c1c83c2](https://github.com/pyroscope-io/pyroscope/commit/c1c83c290997b5b93e86ce7f7c29eaa49d809ee7))


### Features

* **webapp:** add tooltip to main timeline in single view ([#1742](https://github.com/pyroscope-io/pyroscope/issues/1742)) ([508946c](https://github.com/pyroscope-io/pyroscope/commit/508946cf034d2f9aedae03c71429fc05d313897d))



## [0.34.1](https://github.com/pyroscope-io/pyroscope/compare/v0.34.0...v0.34.1) (2022-11-19)


### Bug Fixes

* delete app also deletes metadata ([#1736](https://github.com/pyroscope-io/pyroscope/issues/1736)) ([c485edd](https://github.com/pyroscope-io/pyroscope/commit/c485edd42e68684230f0ec0228b0e7949a5b6bf9))
* **ebpf:** update regexps for sd cgroupv1 matching ([#1719](https://github.com/pyroscope-io/pyroscope/issues/1719)) ([ebc951d](https://github.com/pyroscope-io/pyroscope/commit/ebc951da393c555f2a285074d46dd470ba8c0014))
* make reset view available in sandwich mode context menu ([#1731](https://github.com/pyroscope-io/pyroscope/issues/1731)) ([e41bcaf](https://github.com/pyroscope-io/pyroscope/commit/e41bcaf5cbfc3458958132aa4f54998aedcd8328))
* tag explorer long tag overflow ([#1718](https://github.com/pyroscope-io/pyroscope/issues/1718)) ([b5ee72a](https://github.com/pyroscope-io/pyroscope/commit/b5ee72a651c00dae29e0651ba77e6c587f81c21f))


### Features

* make tag explorer modal adapt to content ([#1733](https://github.com/pyroscope-io/pyroscope/issues/1733)) ([7bdd8a4](https://github.com/pyroscope-io/pyroscope/commit/7bdd8a4f9ab79eeec0fa21afcf5db7c6dcb54fa0))
* pie chart tooltip show units ([#1720](https://github.com/pyroscope-io/pyroscope/issues/1720)) ([8d5d658](https://github.com/pyroscope-io/pyroscope/commit/8d5d65869f8f66833b782e97e45261e460272beb))
* show percentages for diff table instead of absolute values ([#1697](https://github.com/pyroscope-io/pyroscope/issues/1697)) ([71efcb8](https://github.com/pyroscope-io/pyroscope/commit/71efcb868b14ff3771faa5858dd781eba0787bd8))
* **webapp:** filter out apps that are not cpu in exemplars page ([#1722](https://github.com/pyroscope-io/pyroscope/issues/1722)) ([100f943](https://github.com/pyroscope-io/pyroscope/commit/100f943d8b90e294de0ae076ae32f2a9846a1aa2))
* **webapp:** render pie slice label as percent in tag explorer ([#1721](https://github.com/pyroscope-io/pyroscope/issues/1721)) ([79018aa](https://github.com/pyroscope-io/pyroscope/commit/79018aab7f5d089615c29b37a06a30cba237b522))



# [0.34.0](https://github.com/pyroscope-io/pyroscope/compare/v0.33.0...v0.34.0) (2022-11-16)


### Bug Fixes

* add TagsBar component to exemplars single view ([#1696](https://github.com/pyroscope-io/pyroscope/issues/1696)) ([8817502](https://github.com/pyroscope-io/pyroscope/commit/88175023c0db3756a726034b2ea7a5df32a3d25e))
* **backend:** fix cache ([#1664](https://github.com/pyroscope-io/pyroscope/issues/1664)) ([04ab88a](https://github.com/pyroscope-io/pyroscope/commit/04ab88a1d6c7c8fabefe5a46bf69c3137de25d06))
* exemplars page shows weird y-axis numbers ([#1644](https://github.com/pyroscope-io/pyroscope/issues/1644)) ([c672ed7](https://github.com/pyroscope-io/pyroscope/commit/c672ed7d4ba601e3b9e45eaf328260f8f4c812ca))
* heatmap y-axis value with "<" ([#1694](https://github.com/pyroscope-io/pyroscope/issues/1694)) ([76e5748](https://github.com/pyroscope-io/pyroscope/commit/76e574870829082a56963d82c2ede049bba743eb))
* make /apps respect remoteRead ([#1645](https://github.com/pyroscope-io/pyroscope/issues/1645)) ([3e85b17](https://github.com/pyroscope-io/pyroscope/commit/3e85b174b0e3b9960e531d318f0db827ac666290))
* make oauth work when baseUrl is set ([#1673](https://github.com/pyroscope-io/pyroscope/issues/1673)) ([6cc1a2a](https://github.com/pyroscope-io/pyroscope/commit/6cc1a2a75387b0ca4a75ac57191a6a352a072a43))
* **panel-plugin:** remove leaky css ([#1709](https://github.com/pyroscope-io/pyroscope/issues/1709)) ([bc28930](https://github.com/pyroscope-io/pyroscope/commit/bc28930093b0b8494b073a45252ce361dba1a382))
* remove tmp multipart files ([#1678](https://github.com/pyroscope-io/pyroscope/issues/1678)) ([6a1b631](https://github.com/pyroscope-io/pyroscope/commit/6a1b6310e0f51ed1423a87550a296f5e80c8149d))
* sandwich view prompt in comparison view ([#1688](https://github.com/pyroscope-io/pyroscope/issues/1688)) ([5f32774](https://github.com/pyroscope-io/pyroscope/commit/5f3277411c28988d811fa23f1029ff556a434578))
* Update a few function names on comments ([#1676](https://github.com/pyroscope-io/pyroscope/issues/1676)) ([59a339f](https://github.com/pyroscope-io/pyroscope/commit/59a339f2bb7f205dc5b823f80fc71ceab5b28d95))
* **webapp:** make ui consistent when request is cancelled ([#1635](https://github.com/pyroscope-io/pyroscope/issues/1635)) ([d9b8290](https://github.com/pyroscope-io/pyroscope/commit/d9b8290a498fb658b206fc0d6253f6b206e21b9e))
* **webapp:** pass from,until when calling /label{-values} ([#1677](https://github.com/pyroscope-io/pyroscope/issues/1677)) ([a82077d](https://github.com/pyroscope-io/pyroscope/commit/a82077dd23148d839b75b46fb11b6b71492eec99))
* **webapp:** sort appNames alphabetically ([#1655](https://github.com/pyroscope-io/pyroscope/issues/1655)) ([e29d2e2](https://github.com/pyroscope-io/pyroscope/commit/e29d2e29862cf8e21354c194e67f2fd9e376e273))


### Features

* add a generic Tooltip component ([#1643](https://github.com/pyroscope-io/pyroscope/issues/1643)) ([e04a9a5](https://github.com/pyroscope-io/pyroscope/commit/e04a9a5b8abacbf6c3fad87b1a8aaf2ed1636053))
* add Fit Mode to Context Menu ([#1698](https://github.com/pyroscope-io/pyroscope/issues/1698)) ([082a971](https://github.com/pyroscope-io/pyroscope/commit/082a9715d5aed80682fc1f38988a927ca0bbcd93))
* add sandwich view for table/flamegraph ([#1613](https://github.com/pyroscope-io/pyroscope/issues/1613)) ([870c0b8](https://github.com/pyroscope-io/pyroscope/commit/870c0b8f209b7b669b407f6d3b5214876e671d69))
* add single, comparison, diff tabs to heatmap page ([#1672](https://github.com/pyroscope-io/pyroscope/issues/1672)) ([9afe5e5](https://github.com/pyroscope-io/pyroscope/commit/9afe5e564e1fea6fe4026c3180134412ac2930f9))
* disable sandwich view for diff page ([#1693](https://github.com/pyroscope-io/pyroscope/issues/1693)) ([b47b441](https://github.com/pyroscope-io/pyroscope/commit/b47b4411b97167c4d1b85543c0b062f0358310c2))
* enable "reset view" button when table item is highlighted ([#1703](https://github.com/pyroscope-io/pyroscope/issues/1703)) ([7b1bfd5](https://github.com/pyroscope-io/pyroscope/commit/7b1bfd55a3d6186877104e7e164323ee9a4d4f34))
* **flamegraph:** Redesign flamegraph toolbar to allow for more interactions ([#1674](https://github.com/pyroscope-io/pyroscope/issues/1674)) ([646501a](https://github.com/pyroscope-io/pyroscope/commit/646501a3816df6f069454213ac7884198f35cd0b))
* **panel-plugin:** allow setting different views ([#1712](https://github.com/pyroscope-io/pyroscope/issues/1712)) ([058099c](https://github.com/pyroscope-io/pyroscope/commit/058099c857d80c3dc2f1c7c7a99391bd75c72178))
* show gif when heatmap has no selection ([#1658](https://github.com/pyroscope-io/pyroscope/issues/1658)) ([2a3243d](https://github.com/pyroscope-io/pyroscope/commit/2a3243de46d27de5d9cbbc9f50dd1089ba517a5b))
* store application metadata ([#1649](https://github.com/pyroscope-io/pyroscope/issues/1649)) ([eb2d86e](https://github.com/pyroscope-io/pyroscope/commit/eb2d86e17cd966275dad1c8dc0a2c13725e52c8e))
* **webapp:** [notifications] support 'warning' status and arbitrary jsx element ([#1656](https://github.com/pyroscope-io/pyroscope/issues/1656)) ([2ec2b07](https://github.com/pyroscope-io/pyroscope/commit/2ec2b073ffaf7699bdba9d1bc29b3fea8ef4b659))
* **webapp:** Add relative time period dropdown to comparison / diff view ([#1638](https://github.com/pyroscope-io/pyroscope/issues/1638)) ([23cf747](https://github.com/pyroscope-io/pyroscope/commit/23cf7474ec57c920dbc1f230cf3045fdc9a8b305))
* **webapp:** Annotations flot plugin ([#1605](https://github.com/pyroscope-io/pyroscope/issues/1605)) ([fe80686](https://github.com/pyroscope-io/pyroscope/commit/fe80686837953a2f45aa8efc5a7d0a7c0efcc1a8))
* **webapp:** Issue when comparison / diff timelines are out of range ([#1615](https://github.com/pyroscope-io/pyroscope/issues/1615)) ([211ccca](https://github.com/pyroscope-io/pyroscope/commit/211ccca5a03fb78087bdf10b125ad0141b2a5dc9))
* **webapp:** Make explore page show precise numbers in table ([#1695](https://github.com/pyroscope-io/pyroscope/issues/1695)) ([5b47c71](https://github.com/pyroscope-io/pyroscope/commit/5b47c71b6a1e85c39b4ac21b1eaeddf85bad0d94))
* **webapp:** Show top 10 items in Explore page ([#1663](https://github.com/pyroscope-io/pyroscope/issues/1663)) ([73544fb](https://github.com/pyroscope-io/pyroscope/commit/73544fb22f67bda9a27a0519a5b07c45523099a4))



# [0.33.0](https://github.com/pyroscope-io/pyroscope/compare/v0.32.0...v0.33.0) (2022-10-22)


### Bug Fixes

* close databases in deterministic order ([#1623](https://github.com/pyroscope-io/pyroscope/issues/1623)) ([7a8b33c](https://github.com/pyroscope-io/pyroscope/commit/7a8b33cb0c52221836a0c69f8e141705bbdaf627))
* fix node version in update-contributors action ([deb36ad](https://github.com/pyroscope-io/pyroscope/commit/deb36adc5667aa9d53f716f2f3f47482c7cdcdd2))


### Performance Improvements

* optimize allocations in dictionaries ([#1610](https://github.com/pyroscope-io/pyroscope/issues/1610)) ([f8d52f5](https://github.com/pyroscope-io/pyroscope/commit/f8d52f5d478024ad52107a6e1c3510b2a8fabdd6))
* optimize segment key parser allocations ([#1625](https://github.com/pyroscope-io/pyroscope/issues/1625)) ([7c83b32](https://github.com/pyroscope-io/pyroscope/commit/7c83b327e66f1350f5c9255f739c26e2babcaeee))



# [0.32.0](https://github.com/pyroscope-io/pyroscope/compare/v0.31.0...v0.32.0) (2022-10-17)


### Bug Fixes

* disable ptrace capability check for dotnetspy ([#1622](https://github.com/pyroscope-io/pyroscope/issues/1622)) ([68c8cfc](https://github.com/pyroscope-io/pyroscope/commit/68c8cfc5a5e64eb3e818284efcfdd54196673e36))
* make adhoc table fit width ([#1591](https://github.com/pyroscope-io/pyroscope/issues/1591)) ([da74fe8](https://github.com/pyroscope-io/pyroscope/commit/da74fe896536b97911d9cf00a9288c64dda77a28))
* **webapp:** make app selector/timerange dropdowns to be above loading overlay ([#1618](https://github.com/pyroscope-io/pyroscope/issues/1618)) ([f4c8f17](https://github.com/pyroscope-io/pyroscope/commit/f4c8f1716e5a7352ddc4387dbf5613964ccd64e9))



# [0.31.0](https://github.com/pyroscope-io/pyroscope/compare/v0.30.0...v0.31.0) (2022-10-06)


### Bug Fixes

* update regexes for deleted so names ([#1599](https://github.com/pyroscope-io/pyroscope/issues/1599)) ([85d13ea](https://github.com/pyroscope-io/pyroscope/commit/85d13ea06839139040f902dcbee7179db7c41d34))


### Features

* add sorting to explore view table ([#1592](https://github.com/pyroscope-io/pyroscope/issues/1592)) ([64bc4b3](https://github.com/pyroscope-io/pyroscope/commit/64bc4b3ffaaa72ac01513093a74919c3389c45af))
* support speedscope format ([#1589](https://github.com/pyroscope-io/pyroscope/issues/1589)) ([edb5e18](https://github.com/pyroscope-io/pyroscope/commit/edb5e18edb54c5bba8d4af7f4200e765fd57305b))



# [0.30.0](https://github.com/pyroscope-io/pyroscope/compare/v0.29.0...v0.30.0) (2022-10-04)


### Bug Fixes

* **backend:** don't set a default annotation timestamp in controller ([#1504](https://github.com/pyroscope-io/pyroscope/issues/1504)) ([c2b2cfe](https://github.com/pyroscope-io/pyroscope/commit/c2b2cfea9b7be78177ec962ae659ef4a9e688d5a))
* **backend:** upsert annotations ([#1508](https://github.com/pyroscope-io/pyroscope/issues/1508)) ([a557183](https://github.com/pyroscope-io/pyroscope/commit/a557183e773dbcc669f9ab32b8bacae9d7cbbce9))
* export dropdown should close when clicking outside ([#1579](https://github.com/pyroscope-io/pyroscope/issues/1579)) ([b4074a7](https://github.com/pyroscope-io/pyroscope/commit/b4074a72f0482cd59e53960ce0ba437052475b50))
* heatmap bug fixes ([#1545](https://github.com/pyroscope-io/pyroscope/issues/1545)) ([3218c62](https://github.com/pyroscope-io/pyroscope/commit/3218c6295383fa608bfe6fed40501949d1189708))
* merge zstd lib generated so names for jfr ([#1569](https://github.com/pyroscope-io/pyroscope/issues/1569)) ([00ed85a](https://github.com/pyroscope-io/pyroscope/commit/00ed85a351e2e38517a0ad31da5e9ee63168b243))
* tag explorer modal should close when another one is clicked ([#1578](https://github.com/pyroscope-io/pyroscope/issues/1578)) ([c2d1e96](https://github.com/pyroscope-io/pyroscope/commit/c2d1e966e33aed1d483bcabdbfc05a545518a302))
* Update flask rideshare app name ([fdd7b4c](https://github.com/pyroscope-io/pyroscope/commit/fdd7b4c0df2764158cf0e035dff6b8be8d5e0eab))
* **webapp:** annotation doesn't have a weird marking anymore ([#1512](https://github.com/pyroscope-io/pyroscope/issues/1512)) ([20abd58](https://github.com/pyroscope-io/pyroscope/commit/20abd58350a0fd729ac13cb2537c1f7b424178c8))
* **webapp:** don't render popover outside the visible window ([#1534](https://github.com/pyroscope-io/pyroscope/issues/1534)) ([0ce4e7d](https://github.com/pyroscope-io/pyroscope/commit/0ce4e7de968bb3e919fab0dcd7394350eb4716b5))
* **webapp:** format annotation using timezone ([#1522](https://github.com/pyroscope-io/pyroscope/issues/1522)) ([bc68da3](https://github.com/pyroscope-io/pyroscope/commit/bc68da3757154656fe8a799a8170928686305f68))
* **webapp:** show annotations tooltip only when hovering close to the marker ([#1510](https://github.com/pyroscope-io/pyroscope/issues/1510)) ([e8bdf4a](https://github.com/pyroscope-io/pyroscope/commit/e8bdf4abbff10efcdc317aea09d45ecd795e2f42))


### Features

* add 'perf script' format upload ([#1499](https://github.com/pyroscope-io/pyroscope/issues/1499)) ([c7bb5ca](https://github.com/pyroscope-io/pyroscope/commit/c7bb5caf41cb7142d933e0b8f404661fea9f67ef))
* add "Count" title above heatmap scale ([#1571](https://github.com/pyroscope-io/pyroscope/issues/1571)) ([910c496](https://github.com/pyroscope-io/pyroscope/commit/910c496a88c81d03efdc91548c4365dcff3a670a))
* Add count and latency to heatmap tooltip ([#1582](https://github.com/pyroscope-io/pyroscope/issues/1582)) ([4590901](https://github.com/pyroscope-io/pyroscope/commit/459090179b623e5f40d7c31bbfb1bae8c4e9146f))
* add table view to heatmap flamegraph ([#1574](https://github.com/pyroscope-io/pyroscope/issues/1574)) ([e55cc8a](https://github.com/pyroscope-io/pyroscope/commit/e55cc8aff22c863d8aab6395c1b895aee1c46278))
* add ticks to x and y-axis ([#1558](https://github.com/pyroscope-io/pyroscope/issues/1558)) ([4a3e140](https://github.com/pyroscope-io/pyroscope/commit/4a3e1402ff1f117f3a806e4ef4cae456fe60c336))
* **annotations:** allow creating multiple annotations for different apps ([#1562](https://github.com/pyroscope-io/pyroscope/issues/1562)) ([1986b11](https://github.com/pyroscope-io/pyroscope/commit/1986b11f9ca91d4db76ac95368c0e424211da850))
* change color of the selected area of the heatmap ([#1572](https://github.com/pyroscope-io/pyroscope/issues/1572)) ([9ebdded](https://github.com/pyroscope-io/pyroscope/commit/9ebdded1524684f405ad79a5100cde52a2548a0a))
* heatmap improvements ([#1501](https://github.com/pyroscope-io/pyroscope/issues/1501)) ([e9e5bfd](https://github.com/pyroscope-io/pyroscope/commit/e9e5bfdd1e93efaa0428d0a2d3803652d8c7b083))
* Heatmap should show error message if no data is returned ([#1565](https://github.com/pyroscope-io/pyroscope/issues/1565)) ([fe32a07](https://github.com/pyroscope-io/pyroscope/commit/fe32a07e6e1bf05b11a6634f6eea0b44bf8b0536))
* remove heatmap grid ([#1567](https://github.com/pyroscope-io/pyroscope/issues/1567)) ([541133b](https://github.com/pyroscope-io/pyroscope/commit/541133b54d7d10cde0c3815c17077e4e2de1109d))
* show heatmap y-axis units ([#1559](https://github.com/pyroscope-io/pyroscope/issues/1559)) ([8199170](https://github.com/pyroscope-io/pyroscope/commit/81991706e0ba6767da9d9fdefcd89a65f527722a))
* **webapp:** annotations UI ([#1489](https://github.com/pyroscope-io/pyroscope/issues/1489)) ([0e57137](https://github.com/pyroscope-io/pyroscope/commit/0e571379e1bb4f99bfdb9c3a22d1b2827128e3a5))
* **webapp:** create annotations via ui ([#1524](https://github.com/pyroscope-io/pyroscope/issues/1524)) ([53836ce](https://github.com/pyroscope-io/pyroscope/commit/53836ce6b2f4a5eb2debc6a2e03c0b39812bf488))



# [0.29.0](https://github.com/pyroscope-io/pyroscope/compare/v0.28.1...v0.29.0) (2022-09-14)


### Bug Fixes

* add pprof format during ingestion to RawProfile ([#1482](https://github.com/pyroscope-io/pyroscope/issues/1482)) ([b8f0296](https://github.com/pyroscope-io/pyroscope/commit/b8f0296e70b5eebcabdc877ae437fa64ee3a2b41))
* ignore missing heatmap params ([#1480](https://github.com/pyroscope-io/pyroscope/issues/1480)) ([c7e051b](https://github.com/pyroscope-io/pyroscope/commit/c7e051b79481aa398729e034dfa7d44ba44bd4a2))
* **pprof:** multipart upload pprof format ([#1483](https://github.com/pyroscope-io/pyroscope/issues/1483)) ([47d2bcc](https://github.com/pyroscope-io/pyroscope/commit/47d2bccab52fd21731fa28f245a61c0184a9ef1e))
* support for float parameters in exemplar query params ([#1479](https://github.com/pyroscope-io/pyroscope/issues/1479)) ([b8bd9b7](https://github.com/pyroscope-io/pyroscope/commit/b8bd9b7f55fa609b98f406acc83a37378a7885b3))
* **webapp:** CollapseBox overflow ([#1490](https://github.com/pyroscope-io/pyroscope/issues/1490)) ([0af9f1d](https://github.com/pyroscope-io/pyroscope/commit/0af9f1d0ec3f18f2b4cb366eb0b8afda3e2e8de5))
* **webapp:** hide tooltip if there's no data ([#1472](https://github.com/pyroscope-io/pyroscope/issues/1472)) ([df4c8cc](https://github.com/pyroscope-io/pyroscope/commit/df4c8cc360877b7100d987513beae905bc735a74))


### Features

* **backend:** annotations ([#1473](https://github.com/pyroscope-io/pyroscope/issues/1473)) ([9b94f58](https://github.com/pyroscope-io/pyroscope/commit/9b94f58409915f121f20238adbc9d1dc679a00ba))
* **webapp:** Add <CollapseBox /> component ([#1474](https://github.com/pyroscope-io/pyroscope/issues/1474)) ([6595794](https://github.com/pyroscope-io/pyroscope/commit/659579448bdf3e32a77f047aceffa01108d784b3))
* **webapp:** Add Explore Page piechart ([#1477](https://github.com/pyroscope-io/pyroscope/issues/1477)) ([c421b42](https://github.com/pyroscope-io/pyroscope/commit/c421b4225baaa7543b22185c3b26fcf48a60a024))
* **webapp:** Add tracing page with heatmap ([#1433](https://github.com/pyroscope-io/pyroscope/issues/1433)) ([587379a](https://github.com/pyroscope-io/pyroscope/commit/587379aa5067521f38bc892c364ccff6e1d35b28))



## [0.28.1](https://github.com/pyroscope-io/pyroscope/compare/v0.28.0...v0.28.1) (2022-09-06)


### Bug Fixes

* **flamegraph:** add color to tooltip ([#1468](https://github.com/pyroscope-io/pyroscope/issues/1468)) ([1c29ef6](https://github.com/pyroscope-io/pyroscope/commit/1c29ef6328fb4f01389560a4d53f729bbca88ff3))
* **flamegraph:** table width ([#1463](https://github.com/pyroscope-io/pyroscope/issues/1463)) ([f19b8ac](https://github.com/pyroscope-io/pyroscope/commit/f19b8ac778452ea86261563aa1d165dbc2d089e7))
* **flamegraph:** table width ([#1466](https://github.com/pyroscope-io/pyroscope/issues/1466)) ([a60f608](https://github.com/pyroscope-io/pyroscope/commit/a60f608475a9caea0e1910f30d1c663fd38cee96))


### Features

* **webapp:** Add tooltip in explore timeline ([#1422](https://github.com/pyroscope-io/pyroscope/issues/1422)) ([b5ce89a](https://github.com/pyroscope-io/pyroscope/commit/b5ce89a680256a7d6ff76e45ec38abc721df9a89))



# [0.28.0](https://github.com/pyroscope-io/pyroscope/compare/v0.27.0...v0.28.0) (2022-09-06)


### Bug Fixes

* ebpf rename labels ([#1441](https://github.com/pyroscope-io/pyroscope/issues/1441)) ([a0359dd](https://github.com/pyroscope-io/pyroscope/commit/a0359dd1afa164decaecbe17922254fa41a7338d))
* **flamegraph:** fixed tooltip display with color blind palette ([#1442](https://github.com/pyroscope-io/pyroscope/issues/1442)) ([702ad8b](https://github.com/pyroscope-io/pyroscope/commit/702ad8b937aa05e12e4dc21114c63febd93bf4c2))
* **flamegraph:** table, buttons colors for light mode ([#1458](https://github.com/pyroscope-io/pyroscope/issues/1458)) ([37afd3b](https://github.com/pyroscope-io/pyroscope/commit/37afd3bbdad6b9165143b92dd2312d6b125140f4))
* pprof parser formatting for rbspy ([#1454](https://github.com/pyroscope-io/pyroscope/issues/1454)) ([ca93c31](https://github.com/pyroscope-io/pyroscope/commit/ca93c310eec2a8cf83be8a5ee3cc0aa374e6d975))


### Features

* concurrent storage put ([#1304](https://github.com/pyroscope-io/pyroscope/issues/1304)) ([ec5f8b6](https://github.com/pyroscope-io/pyroscope/commit/ec5f8b66c8a49e7d53767e43564ba25b59628ba5))
* ebpf go symbols - resolve from .gopclntab ([#1447](https://github.com/pyroscope-io/pyroscope/issues/1447)) ([ae78c42](https://github.com/pyroscope-io/pyroscope/commit/ae78c42802f56c36d4fee80a9ddd2cd446d541a2))
* **flamegraph:** added sub-second units support for trace visualization ([#1418](https://github.com/pyroscope-io/pyroscope/issues/1418)) ([21f6550](https://github.com/pyroscope-io/pyroscope/commit/21f6550bf7e280e7ee272982f9ab521bf30683c6))
* **webapp:** display timer for notifications ([#1457](https://github.com/pyroscope-io/pyroscope/issues/1457)) ([b158f38](https://github.com/pyroscope-io/pyroscope/commit/b158f38592b0b1df9c9c1ff025577175075b897e))
* **webapp:** dropdown component for head-first dropdown ([#1435](https://github.com/pyroscope-io/pyroscope/issues/1435)) ([a7d6891](https://github.com/pyroscope-io/pyroscope/commit/a7d6891c8b63d67dc197d7c52812bd946ca688e5))



# [0.27.0](https://github.com/pyroscope-io/pyroscope/compare/v0.26.0...v0.27.0) (2022-08-24)


### Bug Fixes

* **adhoc:** support malformed profiles ([#1438](https://github.com/pyroscope-io/pyroscope/issues/1438)) ([b24d483](https://github.com/pyroscope-io/pyroscope/commit/b24d4838b88f45992be2e04153e55fde98a1b04a))
* do not use sampling ticker for resettable spies ([#1429](https://github.com/pyroscope-io/pyroscope/issues/1429)) ([6f279e4](https://github.com/pyroscope-io/pyroscope/commit/6f279e4fbdabc1dd6db650a5a9139e88d07c4070))
* **ebpf:** ignore pods with empty containerID ([#1437](https://github.com/pyroscope-io/pyroscope/issues/1437)) ([937f9b9](https://github.com/pyroscope-io/pyroscope/commit/937f9b9946e6f7dbe1cf984a6a20000fb09a066c))
* **webapp:** fixes auth ([#1434](https://github.com/pyroscope-io/pyroscope/issues/1434)) ([a70ca26](https://github.com/pyroscope-io/pyroscope/commit/a70ca269eb02983248d34b46bc9bbd9c596a0da6))


### Features

* **webapp:** Add settings/apps page ([#1424](https://github.com/pyroscope-io/pyroscope/issues/1424)) ([f87ce69](https://github.com/pyroscope-io/pyroscope/commit/f87ce69d8af890402d5845f7d3047e393682c934))



# [0.26.0](https://github.com/pyroscope-io/pyroscope/compare/v0.25.1...v0.26.0) (2022-08-19)


### Bug Fixes

* Add different color palette for Explore Timeline ([#1399](https://github.com/pyroscope-io/pyroscope/issues/1399)) ([5e129e0](https://github.com/pyroscope-io/pyroscope/commit/5e129e0552422ca3b36c85153248c1d8278fc0de))
* **flamegraph:** fix its styling ([#1388](https://github.com/pyroscope-io/pyroscope/issues/1388)) ([5788cf9](https://github.com/pyroscope-io/pyroscope/commit/5788cf9943a579acd35de224537016488a98808f))
* Make trace units be time based ([#1387](https://github.com/pyroscope-io/pyroscope/issues/1387)) ([a567c2c](https://github.com/pyroscope-io/pyroscope/commit/a567c2c1877c462a6a4dceda87455017c1cf7276))
* **webapp:** refresh data when (re)submitting query ([#1410](https://github.com/pyroscope-io/pyroscope/issues/1410)) ([3554f84](https://github.com/pyroscope-io/pyroscope/commit/3554f84060ab54836e9ae1f2b49f9174c3e66041)), closes [#1406](https://github.com/pyroscope-io/pyroscope/issues/1406)
* **webapp:** return empty string when range doesn't make sense ([#1419](https://github.com/pyroscope-io/pyroscope/issues/1419)) ([6f69c6f](https://github.com/pyroscope-io/pyroscope/commit/6f69c6f5209a12e1d8c54e962fcba6dc373bd7ff))


### Features

* **adhoc:** support passing custom spyName/unit when uploading ([#1417](https://github.com/pyroscope-io/pyroscope/issues/1417)) ([9cc0f39](https://github.com/pyroscope-io/pyroscope/commit/9cc0f398519f650154c00109633f7bd1b82bc0e8))
* **ebpf:** pythonless, portable ebpf ([#1314](https://github.com/pyroscope-io/pyroscope/issues/1314)) ([f124c46](https://github.com/pyroscope-io/pyroscope/commit/f124c46c5fb0f7fde13ad28af735d3018ecd393c))
* reuse Table component everywhere ([#1403](https://github.com/pyroscope-io/pyroscope/issues/1403)) ([a79f61b](https://github.com/pyroscope-io/pyroscope/commit/a79f61b39d8ae5b199710e79dc05e9352044b3b4))
* support high number of series in explore view timeline ([#1384](https://github.com/pyroscope-io/pyroscope/issues/1384)) ([482e23e](https://github.com/pyroscope-io/pyroscope/commit/482e23e52f006a201e40a2b4ff6092e0f246aa03))
* **webapp:** set title automatically ([#1397](https://github.com/pyroscope-io/pyroscope/issues/1397)) ([74821ca](https://github.com/pyroscope-io/pyroscope/commit/74821ca0f0e3ebec38c83f83bf54318d931f66f6))
* **webapp:** update timeline appearance and refactor flot plugins ([#1323](https://github.com/pyroscope-io/pyroscope/issues/1323)) ([9393449](https://github.com/pyroscope-io/pyroscope/commit/9393449fdabd0cb38c4fae87b3ba0ce73251b41d))



## [0.25.1](https://github.com/pyroscope-io/pyroscope/compare/v0.25.0...v0.25.1) (2022-08-08)


### Bug Fixes

* export flamegraph "head" select styles ([#1367](https://github.com/pyroscope-io/pyroscope/issues/1367)) ([7bc8b3e](https://github.com/pyroscope-io/pyroscope/commit/7bc8b3e9d41faf8ac0422764d36406b8b890870b))
* make table value say 0 when it is 0 ([#1371](https://github.com/pyroscope-io/pyroscope/issues/1371)) ([30067ad](https://github.com/pyroscope-io/pyroscope/commit/30067ad5da6eec7655d99b19aef0c012be923853))


### Features

* add loader to tag exlorer page ([#1368](https://github.com/pyroscope-io/pyroscope/issues/1368)) ([16a727d](https://github.com/pyroscope-io/pyroscope/commit/16a727d5f51847114fe360eb70aff1bf9bfd3a9e))
* add ProfilerTable arrow color ([#1366](https://github.com/pyroscope-io/pyroscope/issues/1366)) ([be3901d](https://github.com/pyroscope-io/pyroscope/commit/be3901da82fe1fbed54171938f2bef638819319b))



# [0.25.0](https://github.com/pyroscope-io/pyroscope/compare/v0.24.0...v0.25.0) (2022-08-08)


### Bug Fixes

* admin controller test compilation ([#1322](https://github.com/pyroscope-io/pyroscope/issues/1322)) ([60299ed](https://github.com/pyroscope-io/pyroscope/commit/60299ed4196241ff185e3a38478527cd2cc59a29))


### Features

* add app dropdown footer ([#1340](https://github.com/pyroscope-io/pyroscope/issues/1340)) ([dc07d04](https://github.com/pyroscope-io/pyroscope/commit/dc07d04e28cdd821829eaf211a8543723b3956aa))
* add tooltip table text for when units is undefined ([#1341](https://github.com/pyroscope-io/pyroscope/issues/1341)) ([a9fd5ac](https://github.com/pyroscope-io/pyroscope/commit/a9fd5ac43f6429c70ccbc70cb8bf89c581e915fb))
* **api:** Implement delete app functionality as an HTTP endpoint backend [#1223](https://github.com/pyroscope-io/pyroscope/issues/1223)  ([#1239](https://github.com/pyroscope-io/pyroscope/issues/1239)) ([b82f426](https://github.com/pyroscope-io/pyroscope/commit/b82f42625f5ee80fad9701d1abd2e0b059aee46c))
* configurable disk alert threshold ([#1318](https://github.com/pyroscope-io/pyroscope/issues/1318)) ([7281bd8](https://github.com/pyroscope-io/pyroscope/commit/7281bd8c81aa7fa6a22875c474529b0450f0eb16))
* create tag-explorer page for analyzing tag breakdowns ([#1293](https://github.com/pyroscope-io/pyroscope/issues/1293)) ([5456a86](https://github.com/pyroscope-io/pyroscope/commit/5456a866cfa6b3800fb7d359ff55032a84129138))
* enhance tag explorer view ([#1329](https://github.com/pyroscope-io/pyroscope/issues/1329)) ([7d66d75](https://github.com/pyroscope-io/pyroscope/commit/7d66d750ba68d27a5046751221dd51d465a08488))
* upload any arbitrary data (collapsed/pprof/json) via adhoc ui ([#1327](https://github.com/pyroscope-io/pyroscope/issues/1327)) ([6620888](https://github.com/pyroscope-io/pyroscope/commit/662088820b430abb2e4dcb9270cbaf62771ca601)), closes [#1333](https://github.com/pyroscope-io/pyroscope/issues/1333) [#1333](https://github.com/pyroscope-io/pyroscope/issues/1333)
* **webapp:** break adhoc ui upload into 2 steps ([#1352](https://github.com/pyroscope-io/pyroscope/issues/1352)) ([9c15298](https://github.com/pyroscope-io/pyroscope/commit/9c15298251048e06a96f738b85f2915326c44f25))
* **webapp:** persist uploaded data via adhoc ui ([#1351](https://github.com/pyroscope-io/pyroscope/issues/1351)) ([cebefc9](https://github.com/pyroscope-io/pyroscope/commit/cebefc94305ffb98467261b24eb6c65ca5e1a18c))



# [0.24.0](https://github.com/pyroscope-io/pyroscope/compare/v0.23.0...v0.24.0) (2022-07-28)


### Bug Fixes

* segment stub deserialization ([#1310](https://github.com/pyroscope-io/pyroscope/issues/1310)) ([1b65b32](https://github.com/pyroscope-io/pyroscope/commit/1b65b32a9b51d40b914c9872b37e9cf8c58987fe))


### chore

* **flamegraph/models:** make it mandatory to handle all spyNames ([#1300](https://github.com/pyroscope-io/pyroscope/issues/1300)) ([f7a95a0](https://github.com/pyroscope-io/pyroscope/commit/f7a95a0225c1a39262962a47fd2a1cd493a8333b))


### Features

* show functions % of total [units] in Table ([#1288](https://github.com/pyroscope-io/pyroscope/issues/1288)) ([6c71195](https://github.com/pyroscope-io/pyroscope/commit/6c71195295c3ed5591917ff754e670d4220b77d0))


### BREAKING CHANGES

* **flamegraph/models:** it will throw an error if spyName is unsupported



# [0.23.0](https://github.com/pyroscope-io/pyroscope/compare/v0.22.1...v0.23.0) (2022-07-25)


### Bug Fixes

* **ebpf:** fix stderr reading deadlock ([#1292](https://github.com/pyroscope-io/pyroscope/issues/1292)) ([c163b37](https://github.com/pyroscope-io/pyroscope/commit/c163b371db12ee658fed8e284bdc166f4eacdd8a))
* exemplars metadata and timestamps ([#1299](https://github.com/pyroscope-io/pyroscope/issues/1299)) ([1a239f2](https://github.com/pyroscope-io/pyroscope/commit/1a239f256a385ebb196cfd8bd9d0764d19b2414c))


### Features

* implements support for environment variable substitutions in config file ([#1283](https://github.com/pyroscope-io/pyroscope/issues/1283)) ([e72c847](https://github.com/pyroscope-io/pyroscope/commit/e72c8477781f528d6f38f4f754e3d76ca6171fad))



## [0.22.1](https://github.com/pyroscope-io/pyroscope/compare/v0.22.0...v0.22.1) (2022-07-19)


### Bug Fixes

* don't write to local db when `disable-local-writes` is set ([#1287](https://github.com/pyroscope-io/pyroscope/issues/1287)) ([4f791e2](https://github.com/pyroscope-io/pyroscope/commit/4f791e25ebec691013b9568956b533b77032d660))



# [0.22.0](https://github.com/pyroscope-io/pyroscope/compare/v0.21.0...v0.22.0) (2022-07-16)


### Bug Fixes

* **frontend:** fix latest version checks ([#1243](https://github.com/pyroscope-io/pyroscope/issues/1243)) ([293078a](https://github.com/pyroscope-io/pyroscope/commit/293078a1bf6e8b8aa0a7e436faaa77bacaaa4b56))
* rideshare nodejs example anonymous functions  ([#1261](https://github.com/pyroscope-io/pyroscope/issues/1261)) ([d0720a8](https://github.com/pyroscope-io/pyroscope/commit/d0720a8336aef203a720c2a745fbaab1d70351dc))


### Features

* add new tooltip design ([#1246](https://github.com/pyroscope-io/pyroscope/issues/1246)) ([8345168](https://github.com/pyroscope-io/pyroscope/commit/83451683d131671771b0e97e052068b08bfe35bd))
* adds support for group-by queries on the backend ([#1244](https://github.com/pyroscope-io/pyroscope/issues/1244)) ([c52f0e4](https://github.com/pyroscope-io/pyroscope/commit/c52f0e4fdc08feced533d60b9daf0c21c565381c))
* **flamegraph:** Add support for visualizing traces ([#1233](https://github.com/pyroscope-io/pyroscope/issues/1233)) ([b15d094](https://github.com/pyroscope-io/pyroscope/commit/b15d094ebb06592a406b4b73485c0f316c411b08))
* **flamegraph:** allow to filter items in table ([#1226](https://github.com/pyroscope-io/pyroscope/issues/1226)) ([e87284d](https://github.com/pyroscope-io/pyroscope/commit/e87284d4d25ae04f2ca50892d4ed89345aa64b3e))
* remote read functionality ([#1253](https://github.com/pyroscope-io/pyroscope/issues/1253)) ([e2af971](https://github.com/pyroscope-io/pyroscope/commit/e2af9717b307a8d2889bd10e53b81fe054c1c661))
* update right-click context menu ([#1259](https://github.com/pyroscope-io/pyroscope/issues/1259)) ([8aea02f](https://github.com/pyroscope-io/pyroscope/commit/8aea02f56320daacfd753d73db6936dcc7cdaef8))



# [0.21.0](https://github.com/pyroscope-io/pyroscope/compare/v0.20.0...v0.21.0) (2022-07-06)


### Bug Fixes

* add sidebar separation lines ([#1216](https://github.com/pyroscope-io/pyroscope/issues/1216)) ([9efc566](https://github.com/pyroscope-io/pyroscope/commit/9efc5666f699a22b6759a326fa663bfe1bd072e3))
* adhoc/diff-view data table initial render ([#1190](https://github.com/pyroscope-io/pyroscope/issues/1190)) ([b03794c](https://github.com/pyroscope-io/pyroscope/commit/b03794cdad8873685cca734dc287c546442bec99))
* colors on login pages ([#1197](https://github.com/pyroscope-io/pyroscope/issues/1197)) ([a6b2b22](https://github.com/pyroscope-io/pyroscope/commit/a6b2b2275a21bd0f82dc8d4c62eeb34c80da9e3f))
* default name when exporting diff ([#1195](https://github.com/pyroscope-io/pyroscope/issues/1195)) ([c8e9b79](https://github.com/pyroscope-io/pyroscope/commit/c8e9b79405be23a760260f40d0e594b8c484f165))
* **flamegraph:** do a deep comparison for whether the flamegraph is the same ([#1212](https://github.com/pyroscope-io/pyroscope/issues/1212)) ([910d8ea](https://github.com/pyroscope-io/pyroscope/commit/910d8eaeab9c23017da26ecc01c527c3b204b88a))
* **frontend:** don't crash when flamegraph changes ([#1200](https://github.com/pyroscope-io/pyroscope/issues/1200)) ([f558e0d](https://github.com/pyroscope-io/pyroscope/commit/f558e0d70e9341d0374dd17c33202e09979b1e38))
* improved nodes coloring by fixing murmur math ([#1214](https://github.com/pyroscope-io/pyroscope/issues/1214)) ([8ea4f73](https://github.com/pyroscope-io/pyroscope/commit/8ea4f730fceb185dba3943dbba524444f2082596))
* load exemplar metadata from segment ([#1185](https://github.com/pyroscope-io/pyroscope/issues/1185)) ([e869730](https://github.com/pyroscope-io/pyroscope/commit/e869730e472c66ee38d66789722c03864e200b83))
* single view app update should change comp/diff view left and right apps ([#1211](https://github.com/pyroscope-io/pyroscope/issues/1211)) ([9a4f34d](https://github.com/pyroscope-io/pyroscope/commit/9a4f34d29090dea456de3014ce2a491e7da83f11))
* Update flamegraph color pallette ([9476039](https://github.com/pyroscope-io/pyroscope/commit/9476039cff2fe5d06b11ad8748517d16b93f1cc1))
* zoom/focus reset on changing selected node [refactored] ([#1184](https://github.com/pyroscope-io/pyroscope/issues/1184)) ([949052d](https://github.com/pyroscope-io/pyroscope/commit/949052d6db23daedde589a6eaa7c06a4db527cab))


### Features

* add adhoc sort by date ([#1187](https://github.com/pyroscope-io/pyroscope/issues/1187)) ([206d2c6](https://github.com/pyroscope-io/pyroscope/commit/206d2c6a6e35d30d85f35a0103b8fb0d71b8c0f5))
* add an explanation for what each API Key Role is for ([#1210](https://github.com/pyroscope-io/pyroscope/issues/1210)) ([88e04f3](https://github.com/pyroscope-io/pyroscope/commit/88e04f34ed99327fbc95b713c2866968e35684d0))
* add titles to charts / flamegraphs ([#1208](https://github.com/pyroscope-io/pyroscope/issues/1208)) ([836fa97](https://github.com/pyroscope-io/pyroscope/commit/836fa97f126f8b7ebfb966bb52a97b5bdf179d83))
* **frontend:** support disabling exporting to flamegraph.com ([#1188](https://github.com/pyroscope-io/pyroscope/issues/1188)) ([cd48732](https://github.com/pyroscope-io/pyroscope/commit/cd48732bb28dfab903ef00799f2bacdb6d991e0d))
* support for micro-, milli-, and nanoseconds ([#1209](https://github.com/pyroscope-io/pyroscope/issues/1209)) ([f1ba768](https://github.com/pyroscope-io/pyroscope/commit/f1ba76848163506a043ec3321a25052f66161bb9))
* **webapp:** new app selector ([#1199](https://github.com/pyroscope-io/pyroscope/issues/1199)) ([d671810](https://github.com/pyroscope-io/pyroscope/commit/d6718109bc307191b7e44e0fd0c072958d5e0cc2))



# [0.20.0](https://github.com/pyroscope-io/pyroscope/compare/v0.19.0...v0.20.0) (2022-06-27)


### Bug Fixes

* stack overflow in parser callback ([#1174](https://github.com/pyroscope-io/pyroscope/issues/1174)) ([c70a643](https://github.com/pyroscope-io/pyroscope/commit/c70a6433b5526d1d075c434092209b1ecac82353))


### Features

* adds proper support for goroutines, block, mutex profiling ([#1178](https://github.com/pyroscope-io/pyroscope/issues/1178)) ([b2e680c](https://github.com/pyroscope-io/pyroscope/commit/b2e680cfbf3c24856543f3a5478204cc24d7cbf7))
* AWS EC2 service discovery ([d02851c](https://github.com/pyroscope-io/pyroscope/commit/d02851c3f594da6243fb6e81e3155843fc87b3ed))
* **self-profiling:** allow tags to be set ([#1158](https://github.com/pyroscope-io/pyroscope/issues/1158)) ([ac855ba](https://github.com/pyroscope-io/pyroscope/commit/ac855ba7629a6a974f461347ad3291fa6f2e2eeb))



# [0.19.0](https://github.com/pyroscope-io/pyroscope/compare/v0.18.0...v0.19.0) (2022-06-13)


### Bug Fixes

* Fix missed style in tags submenu ([#1154](https://github.com/pyroscope-io/pyroscope/issues/1154)) ([006771b](https://github.com/pyroscope-io/pyroscope/commit/006771b4fa541289b1dec180e477ce3130e8ffd8))
* fix regexp typo ([#1151](https://github.com/pyroscope-io/pyroscope/issues/1151)) ([6396017](https://github.com/pyroscope-io/pyroscope/commit/6396017b09c0dfe731b35925ea5ba9459888e922))
* infinite loop when no apps are available ([#1125](https://github.com/pyroscope-io/pyroscope/issues/1125)) ([330eb23](https://github.com/pyroscope-io/pyroscope/commit/330eb234a1f2b7b3de4b36f862729180462262ce))
* make pprof parser to track the sequence ([#1139](https://github.com/pyroscope-io/pyroscope/issues/1139)) ([e448205](https://github.com/pyroscope-io/pyroscope/commit/e448205a8e3f4320c9bca29261a5a7bc7bc8c896))
* Merge jvm generated classes in jfr at ingestion time ([#1149](https://github.com/pyroscope-io/pyroscope/issues/1149)) ([c80878f](https://github.com/pyroscope-io/pyroscope/commit/c80878f765c6f1f8bcbe0d26eebd83d117f55113))
* pprof parser sample types config ([#1145](https://github.com/pyroscope-io/pyroscope/issues/1145)) ([efcb0bb](https://github.com/pyroscope-io/pyroscope/commit/efcb0bb6eb1bd2c8ece67c01d049e4b27b72229c))
* still write to local storage when remote write is turned on ([#1144](https://github.com/pyroscope-io/pyroscope/issues/1144)) ([baba1b8](https://github.com/pyroscope-io/pyroscope/commit/baba1b88dcb876cbe6bb335032d4a64668b5dbde))
* use new wg per parallelizer request ([#1138](https://github.com/pyroscope-io/pyroscope/issues/1138)) ([7757c44](https://github.com/pyroscope-io/pyroscope/commit/7757c447b5f982f234797851869ce917c8dfa45f))
* **webapp:** fix border of <input> element ([#1127](https://github.com/pyroscope-io/pyroscope/issues/1127)) ([458b62b](https://github.com/pyroscope-io/pyroscope/commit/458b62bcbd50ecc612636565c6dfe821b395fd87))


### Features

* Add Ability to Sync Search Bar in Comparison View ([#1120](https://github.com/pyroscope-io/pyroscope/issues/1120)) ([8300792](https://github.com/pyroscope-io/pyroscope/commit/830079299cef97db33d26ada31cbdccbc00e3268))
* allow skipping exemplars in pprof profiles ([#1146](https://github.com/pyroscope-io/pyroscope/issues/1146)) ([ff4e030](https://github.com/pyroscope-io/pyroscope/commit/ff4e030b4f86a0f725b719fdce328c1fc917be49))
* multiple remote write targets ([#1135](https://github.com/pyroscope-io/pyroscope/issues/1135)) ([75dea47](https://github.com/pyroscope-io/pyroscope/commit/75dea471205a1055549fd703f44efeb20dc1a5b9))
* remote write ([#1122](https://github.com/pyroscope-io/pyroscope/issues/1122)) ([e8d3d24](https://github.com/pyroscope-io/pyroscope/commit/e8d3d2457d6bc48bc8a5983d4d066b9f0ebc7b73))



# [0.18.0](https://github.com/pyroscope-io/pyroscope/compare/v0.17.1...v0.18.0) (2022-06-06)


### Bug Fixes

* flamegraph palette selector button styles ([#1113](https://github.com/pyroscope-io/pyroscope/issues/1113)) ([d7a7b11](https://github.com/pyroscope-io/pyroscope/commit/d7a7b117c13beb9528e730bec1353efe72767f83))
* flamegraph palette selector checkmark styles ([#1114](https://github.com/pyroscope-io/pyroscope/issues/1114)) ([755893f](https://github.com/pyroscope-io/pyroscope/commit/755893f23a04c1031a858c39e8729a5074eaf67b))


### Features

* add support for labels when used with JFR and async-profiler's contextId ([#1096](https://github.com/pyroscope-io/pyroscope/issues/1096)) ([b5a57a3](https://github.com/pyroscope-io/pyroscope/commit/b5a57a36cd8382d938b1bf13e06f495ae944f16b))
* Color mode ([#1103](https://github.com/pyroscope-io/pyroscope/issues/1103)) ([8855859](https://github.com/pyroscope-io/pyroscope/commit/885585958012775f0d51ea82208d641d10215574))
* UTC timezone ([#1107](https://github.com/pyroscope-io/pyroscope/issues/1107)) ([9fa550c](https://github.com/pyroscope-io/pyroscope/commit/9fa550c0b577625780aeb00b5fcd9ca3858d410a))



## [0.17.1](https://github.com/pyroscope-io/pyroscope/compare/v0.17.0...v0.17.1) (2022-05-26)


### Bug Fixes

* jfr ingestion issue in 0.17.0 ([#1112](https://github.com/pyroscope-io/pyroscope/issues/1112)) ([0f93772](https://github.com/pyroscope-io/pyroscope/commit/0f93772554817544125c79194a996a4ef61d4d44))



# [0.17.0](https://github.com/pyroscope-io/pyroscope/compare/v0.16.0...v0.17.0) (2022-05-26)


### Bug Fixes

* **flamegraph:** don't ship react-dom ([#1102](https://github.com/pyroscope-io/pyroscope/issues/1102)) ([c80240c](https://github.com/pyroscope-io/pyroscope/commit/c80240cbfda4d0573baf05b9c48d5b658791bffd))
* issue where apps show up without profile type ([#1110](https://github.com/pyroscope-io/pyroscope/issues/1110)) ([4a1ffb1](https://github.com/pyroscope-io/pyroscope/commit/4a1ffb1f102a4c2d5a54013052acf96c3aef498f))
* **webapp:** Add minimum width for "select tag" dropdown [#1065](https://github.com/pyroscope-io/pyroscope/issues/1065) ([#1109](https://github.com/pyroscope-io/pyroscope/issues/1109)) ([ab47ad5](https://github.com/pyroscope-io/pyroscope/commit/ab47ad52047b03fc3df42126cc178dd733d6471b))


### Performance Improvements

* speeds up jfr parsing by updating jfr parser version ([#1111](https://github.com/pyroscope-io/pyroscope/issues/1111)) ([e31d65c](https://github.com/pyroscope-io/pyroscope/commit/e31d65caaf6e02dbc519e5fcdf1601e4341b8527))



# [0.16.0](https://github.com/pyroscope-io/pyroscope/compare/v0.15.4...v0.16.0) (2022-05-12)


### Bug Fixes

* adding master key ([6ae1e18](https://github.com/pyroscope-io/pyroscope/commit/6ae1e1813863b9822c00adc5115bb230f12f1c0b))
* updates otelpyroscope in the jaeger example ([#1064](https://github.com/pyroscope-io/pyroscope/issues/1064)) ([d2af864](https://github.com/pyroscope-io/pyroscope/commit/d2af864c456d70ed785485eba37738d4c7c68501))


### Features

* **flamegraph:** User should be able to adjust title visibility over the Flamegraph ([#1073](https://github.com/pyroscope-io/pyroscope/issues/1073)) ([bd74aae](https://github.com/pyroscope-io/pyroscope/commit/bd74aae448f3d30398484d589675ea168d816a70))
* **frontend:** allow copying notification message ([#1086](https://github.com/pyroscope-io/pyroscope/issues/1086)) ([d30b787](https://github.com/pyroscope-io/pyroscope/commit/d30b78773ad58ec0aceadf40e3ba25900bc4971b))
* **integrations:** nodejs support ([#1089](https://github.com/pyroscope-io/pyroscope/issues/1089)) ([c4b4164](https://github.com/pyroscope-io/pyroscope/commit/c4b41645fa9e44f4c41b909b4671e2bae6d36f55))
* nodejs push & pull mode ([#1060](https://github.com/pyroscope-io/pyroscope/issues/1060)) ([4317103](https://github.com/pyroscope-io/pyroscope/commit/4317103354b5712c561e4cead7f6906c21a3005c))



## [0.15.4](https://github.com/pyroscope-io/pyroscope/compare/v0.15.3...v0.15.4) (2022-04-27)


### Bug Fixes

* **webapp:** tag selector overflow ([#1055](https://github.com/pyroscope-io/pyroscope/issues/1055)) ([f7c7917](https://github.com/pyroscope-io/pyroscope/commit/f7c79179c323b95b8966a12729e1091e4a57dc0f))


### Reverts

* Revert "fix(flamegraph): fix table contrast (#1053)" (#1063) ([a4dd7f6](https://github.com/pyroscope-io/pyroscope/commit/a4dd7f6417b9c37134c9a63143a1d1f8ccb2ee3d)), closes [#1053](https://github.com/pyroscope-io/pyroscope/issues/1053) [#1063](https://github.com/pyroscope-io/pyroscope/issues/1063)



## [0.15.3](https://github.com/pyroscope-io/pyroscope/compare/v0.15.2...v0.15.3) (2022-04-27)


### Features

* rails example added ([#1041](https://github.com/pyroscope-io/pyroscope/issues/1041)) ([a722a6e](https://github.com/pyroscope-io/pyroscope/commit/a722a6e93fdd1895179a0e1481c5d25a3c0dd5a5))



## [0.15.2](https://github.com/pyroscope-io/pyroscope/compare/v0.15.1...v0.15.2) (2022-04-24)


### Bug Fixes

* **flamegraph:** fix table contrast ([#1053](https://github.com/pyroscope-io/pyroscope/issues/1053)) ([6246f21](https://github.com/pyroscope-io/pyroscope/commit/6246f211967d073febdca8fb578bb805b96597cd))
* **jfr:** fixes a parser regression introduced in 1.15.0 ([#1050](https://github.com/pyroscope-io/pyroscope/issues/1050)) ([946468d](https://github.com/pyroscope-io/pyroscope/commit/946468dbf42ff4450edc94762812ddb4a5f3482d))
* remove force compation ([#1036](https://github.com/pyroscope-io/pyroscope/issues/1036)) ([d1b1547](https://github.com/pyroscope-io/pyroscope/commit/d1b1547edbc61537537add4f614c4ada67a83335))



## [0.15.1](https://github.com/pyroscope-io/pyroscope/compare/v0.15.0...v0.15.1) (2022-04-19)


### Bug Fixes

* delete data in batches instead of using badgerDB drop prefix ([#1035](https://github.com/pyroscope-io/pyroscope/issues/1035)) ([10e7006](https://github.com/pyroscope-io/pyroscope/commit/10e7006d8fe84a3c13e90fdc3edffbde481f6c7e))


### Features

* move login to react ([#1031](https://github.com/pyroscope-io/pyroscope/issues/1031)) ([1cb6f9a](https://github.com/pyroscope-io/pyroscope/commit/1cb6f9a08d825acf643b5ef8b51cecab338b1314)), closes [#985](https://github.com/pyroscope-io/pyroscope/issues/985) [#991](https://github.com/pyroscope-io/pyroscope/issues/991)
* **webapp:** add modal for custom export name ([#965](https://github.com/pyroscope-io/pyroscope/issues/965)) ([422ea82](https://github.com/pyroscope-io/pyroscope/commit/422ea82fd20a64eda44ae62821f67a89c129a594))



# [0.15.0](https://github.com/pyroscope-io/pyroscope/compare/v0.14.0...v0.15.0) (2022-04-14)


### Bug Fixes

* **flamegraph:** inject its styles via css only ([#1023](https://github.com/pyroscope-io/pyroscope/issues/1023)) ([c20a137](https://github.com/pyroscope-io/pyroscope/commit/c20a137a56141f944967c8e229c16c773ec4a607))
* **webapp:** service-discovery ([#1016](https://github.com/pyroscope-io/pyroscope/issues/1016)) ([2da460e](https://github.com/pyroscope-io/pyroscope/commit/2da460e57437193138110b73e49a4209b04d9984))


### Features

* add lock profiling support in jfr parser. ([#1015](https://github.com/pyroscope-io/pyroscope/issues/1015)) ([10baacd](https://github.com/pyroscope-io/pyroscope/commit/10baacd24439c30c1bbf5412045eb01aa3abbeea))


### Performance Improvements

* **retention:** improve performance of exemplars removal ([#1018](https://github.com/pyroscope-io/pyroscope/issues/1018)) ([8e7e596](https://github.com/pyroscope-io/pyroscope/commit/8e7e5962eb6479a7bb216d6dc93a022d29b49008)), closes [#962](https://github.com/pyroscope-io/pyroscope/issues/962)



# [0.14.0](https://github.com/pyroscope-io/pyroscope/compare/v0.13.0...v0.14.0) (2022-04-08)


### Bug Fixes

* flaky pprof test ([#990](https://github.com/pyroscope-io/pyroscope/issues/990)) ([044ee75](https://github.com/pyroscope-io/pyroscope/commit/044ee751479145e5d3dca2531df365f13733ac5c))
* **flamegraph:** clicking on anywhere on a row selects that row ([#969](https://github.com/pyroscope-io/pyroscope/issues/969)) ([ee84788](https://github.com/pyroscope-io/pyroscope/commit/ee8478812743e1381818e769706df83506ed6f53))
* **flamegraph:** only show diff options when in diff mode ([#972](https://github.com/pyroscope-io/pyroscope/issues/972)) ([625d4de](https://github.com/pyroscope-io/pyroscope/commit/625d4de340bc576f72b42f9db26605d02bc86c51))
* **pprof:** calculate sample rate based on the profile units ([#992](https://github.com/pyroscope-io/pyroscope/issues/992)) ([c458556](https://github.com/pyroscope-io/pyroscope/commit/c45855676a4f5d1f98ce1e0d1217755b0417f9c9))
* **pull-mode:** aggregation is always sum ([#1001](https://github.com/pyroscope-io/pyroscope/issues/1001)) ([b11d044](https://github.com/pyroscope-io/pyroscope/commit/b11d04447b072d82e7a12684c96650b67407fa99))
* **server:** always returns timeline even if there's not data ([#1012](https://github.com/pyroscope-io/pyroscope/issues/1012)) ([0ecfe03](https://github.com/pyroscope-io/pyroscope/commit/0ecfe0352a8338e9e7d841879cbf7742e6329ed1))


### Features

* **ingestion:** add support for memory allocation events in JFR. ([#961](https://github.com/pyroscope-io/pyroscope/issues/961)) ([312cd8c](https://github.com/pyroscope-io/pyroscope/commit/312cd8ca62cbaeefbaba7845439c8e63b6ecc7ca))
* **jfr:** Split wall events into both CPU and Wall profile types. ([#1002](https://github.com/pyroscope-io/pyroscope/issues/1002)) ([06dabcf](https://github.com/pyroscope-io/pyroscope/commit/06dabcfa51718e0d7bbda0dc62472652f2f6aed9))
* separate retention policy for exemplars ([#971](https://github.com/pyroscope-io/pyroscope/issues/971)) ([06d14cf](https://github.com/pyroscope-io/pyroscope/commit/06d14cf7d3a073fe454ed0e7c3e5c55bc2ca4e9d))
* **webapp:** diff arbitrary apps ([#967](https://github.com/pyroscope-io/pyroscope/issues/967)) ([f7e66f1](https://github.com/pyroscope-io/pyroscope/commit/f7e66f1082bb5e1ae2851b0f37f56f46ed43e5e1))



# [0.13.0](https://github.com/pyroscope-io/pyroscope/compare/v0.12.0...v0.13.0) (2022-03-22)


### Bug Fixes

* allow cache eviction and write-back while purging storage ([#962](https://github.com/pyroscope-io/pyroscope/issues/962)) ([cad1afc](https://github.com/pyroscope-io/pyroscope/commit/cad1afc81530a689a2803593e51b04e9854a6958))
* **frontend:** date range picker styling ([#936](https://github.com/pyroscope-io/pyroscope/issues/936)) ([012eb9f](https://github.com/pyroscope-io/pyroscope/commit/012eb9f1b88b0f490d154c7289089eb136f2d197))


### Features

* **flamegraph:** publish FlamegraphRenderer for nodejs ([#944](https://github.com/pyroscope-io/pyroscope/issues/944)) ([c2a5631](https://github.com/pyroscope-io/pyroscope/commit/c2a56310e4b36bc6823d5f9debe6e7ac07c6b877))
* **ingestion:** initial support for JFR format ingestion. ([#954](https://github.com/pyroscope-io/pyroscope/issues/954)) ([25f96a4](https://github.com/pyroscope-io/pyroscope/commit/25f96a4248651179225fabe63088e39245ced997))
* **webapp:** allow comparing distinct queries/tags ([#942](https://github.com/pyroscope-io/pyroscope/issues/942)) ([4d1307c](https://github.com/pyroscope-io/pyroscope/commit/4d1307c3751b263d88430977f2d87473c8ee280e))
* **webapp:** make search in app/tags selector bar sticky ([#950](https://github.com/pyroscope-io/pyroscope/issues/950)) ([c13ad6a](https://github.com/pyroscope-io/pyroscope/commit/c13ad6af04bdc96bbd980a0d89f8da07681fd321))



# [0.12.0](https://github.com/pyroscope-io/pyroscope/compare/v0.11.1...v0.12.0) (2022-03-10)


### Bug Fixes

* **flamegraph:** rerender when 'profile' changes ([#931](https://github.com/pyroscope-io/pyroscope/issues/931)) ([527ae29](https://github.com/pyroscope-io/pyroscope/commit/527ae29222625ec6c74cda270f2add72027ca1e3))


### Features

* add dedicated profiles storage with support for retention policy ([#925](https://github.com/pyroscope-io/pyroscope/issues/925)) ([7c4996e](https://github.com/pyroscope-io/pyroscope/commit/7c4996e2483268c2ce0795202d4edeb3f6219def))
* **flamegraph:** support a new profile field ([#929](https://github.com/pyroscope-io/pyroscope/issues/929)) ([95abe2a](https://github.com/pyroscope-io/pyroscope/commit/95abe2ae3dc253a25a03eb19a9378d13b85c8f08))



## [0.11.1](https://github.com/pyroscope-io/pyroscope/compare/v0.11.0...v0.11.1) (2022-03-07)


### Bug Fixes

* agent authorization ([#923](https://github.com/pyroscope-io/pyroscope/issues/923)) ([0bd5c70](https://github.com/pyroscope-io/pyroscope/commit/0bd5c70bab3e93140b5401e53bdebf889623714b))



# [0.11.0](https://github.com/pyroscope-io/pyroscope/compare/v0.10.2...v0.11.0) (2022-03-01)


### Bug Fixes

* correct typo in dev Makefile target's depedency. ([#895](https://github.com/pyroscope-io/pyroscope/issues/895)) ([9ec9c0a](https://github.com/pyroscope-io/pyroscope/commit/9ec9c0ab4b68ca18a03e9f206e866ae56226ef10))
* disable pyroscope logo ([#890](https://github.com/pyroscope-io/pyroscope/issues/890)) ([0477cff](https://github.com/pyroscope-io/pyroscope/commit/0477cff8565406c330b48c819c0ed16a69653cee))
* **frontend:** only inline svg if imported via react ([#860](https://github.com/pyroscope-io/pyroscope/issues/860)) ([2f3bdf0](https://github.com/pyroscope-io/pyroscope/commit/2f3bdf0adb278c74819c60b215a41aea4ab417ee))
* incorrect reads when downsampling ([#737](https://github.com/pyroscope-io/pyroscope/issues/737)) ([9f109ee](https://github.com/pyroscope-io/pyroscope/commit/9f109ee2e878a1eeb097fa7aab86655d2b4d09b1))
* **panel-plugin:** don't load CSS file since it's loaded using css modules ([#891](https://github.com/pyroscope-io/pyroscope/issues/891)) ([183eaa0](https://github.com/pyroscope-io/pyroscope/commit/183eaa0e0e719d4f1c408195a2f2b5912b5071d3))
* plural of date picker ([#831](https://github.com/pyroscope-io/pyroscope/issues/831)) ([8bd6eb8](https://github.com/pyroscope-io/pyroscope/commit/8bd6eb840123feb395a77d9077a64215ccf4b286))
* use the provided name when it's not empty in JSON conversion. ([#861](https://github.com/pyroscope-io/pyroscope/issues/861)) ([d1c4066](https://github.com/pyroscope-io/pyroscope/commit/d1c40660186dea825ede65ef55cb0df84189f30c))


### Features

* add an upload diff endpoint for adhoc mode. ([#839](https://github.com/pyroscope-io/pyroscope/issues/839)) ([4a11f7d](https://github.com/pyroscope-io/pyroscope/commit/4a11f7dfd23151beb044a385ea0283142b1a8bd5)), closes [#784](https://github.com/pyroscope-io/pyroscope/issues/784)
* add pprof profiles multiplexing ([#898](https://github.com/pyroscope-io/pyroscope/issues/898)) ([3e5711c](https://github.com/pyroscope-io/pyroscope/commit/3e5711cfbee2b62a3decb56ff78f0d58ef3a62f5))
* add support for auth to grafana datasource plugin ([#844](https://github.com/pyroscope-io/pyroscope/issues/844)) ([8712404](https://github.com/pyroscope-io/pyroscope/commit/87124048c194ae27c975ebffd0589ef4241f2601))
* adds a page with pull-mode targets ([#877](https://github.com/pyroscope-io/pyroscope/issues/877)) ([26c21f2](https://github.com/pyroscope-io/pyroscope/commit/26c21f2d3ecd5b043fe9facdb669dbee7cad6877)), closes [#592](https://github.com/pyroscope-io/pyroscope/issues/592) [#592](https://github.com/pyroscope-io/pyroscope/issues/592)
* extract various components ([#868](https://github.com/pyroscope-io/pyroscope/issues/868)) ([fb4c2fc](https://github.com/pyroscope-io/pyroscope/commit/fb4c2fcb3dc279407685fea5eab9937f2dca1a81))



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

