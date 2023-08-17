# Changelog

## [v3.2.0](https://github.com/k0kubun/pp/tree/v3.2.0)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.1.0...v3.2.0)

**Closed issues:**

- ignore private fields in a struct in pretty print golang [\#74](https://github.com/k0kubun/pp/issues/74)
- Bug?: Colorize map field names/keys using "FieldName" color scheme setting [\#72](https://github.com/k0kubun/pp/issues/72)
- Printf not work correctly [\#70](https://github.com/k0kubun/pp/issues/70)
- How to use the latest version? [\#69](https://github.com/k0kubun/pp/issues/69)
- Please provide a simple way to disable color [\#67](https://github.com/k0kubun/pp/issues/67)
- disable printing of struct metadata [\#66](https://github.com/k0kubun/pp/issues/66)

**Merged pull requests:**

- Expose defaultPrettyPrinter as pp.Default [\#75](https://github.com/k0kubun/pp/pull/75) ([k0kubun](https://github.com/k0kubun))
- Bump github.com/mattn/go-colorable from 0.1.12 to 0.1.13 [\#73](https://github.com/k0kubun/pp/pull/73) ([dependabot[bot]](https://github.com/apps/dependabot))
- doc: fix pp.go arguments word typo [\#68](https://github.com/k0kubun/pp/pull/68) ([rasecoiac03](https://github.com/rasecoiac03))

## [v3.1.0](https://github.com/k0kubun/pp/tree/v3.1.0) (2022-01-06)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.10...v3.1.0)

**Merged pull requests:**

- Use SetDecimalUnit\(true\) by default [\#65](https://github.com/k0kubun/pp/pull/65) ([k0kubun](https://github.com/k0kubun))

## [v3.0.10](https://github.com/k0kubun/pp/tree/v3.0.10) (2022-01-06)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.9...v3.0.10)

**Merged pull requests:**

- Remove k0kubun/colorstring from dependencies [\#64](https://github.com/k0kubun/pp/pull/64) ([k0kubun](https://github.com/k0kubun))

## [v3.0.9](https://github.com/k0kubun/pp/tree/v3.0.9) (2022-01-06)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.8...v3.0.9)

**Merged pull requests:**

- Bump github.com/mattn/go-colorable from 0.1.7 to 0.1.12 [\#63](https://github.com/k0kubun/pp/pull/63) ([dependabot[bot]](https://github.com/apps/dependabot))

## [v3.0.8](https://github.com/k0kubun/pp/tree/v3.0.8) (2021-11-30)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.7...v3.0.8)

**Closed issues:**

- Add option to skip unexported fields [\#59](https://github.com/k0kubun/pp/issues/59)
- Verb '%T' is working incorrectly in formatted print [\#58](https://github.com/k0kubun/pp/issues/58)
- Option to print byte as decimal [\#54](https://github.com/k0kubun/pp/issues/54)
- Option for thousands separator [\#53](https://github.com/k0kubun/pp/issues/53)
- The color part of vim is messy [\#52](https://github.com/k0kubun/pp/issues/52)
- Bug: `printTime` assumes the standard library time [\#47](https://github.com/k0kubun/pp/issues/47)
- Feature request: Add `pp:"noprint"` tag [\#42](https://github.com/k0kubun/pp/issues/42)
- Feature request: omitempty [\#22](https://github.com/k0kubun/pp/issues/22)
- Unable to print invalid address [\#21](https://github.com/k0kubun/pp/issues/21)

**Merged pull requests:**

- build: upgrade `go` directive in `go.mod` to 1.17 [\#62](https://github.com/k0kubun/pp/pull/62) ([Juneezee](https://github.com/Juneezee))
- SetThousandsSeparator [\#61](https://github.com/k0kubun/pp/pull/61) ([mariusgrigoriu](https://github.com/mariusgrigoriu))
- Add ExportedOnly option [\#60](https://github.com/k0kubun/pp/pull/60) ([k0kubun](https://github.com/k0kubun))
- Add SetDecimalUint option [\#55](https://github.com/k0kubun/pp/pull/55) ([k0kubun](https://github.com/k0kubun))
- Add Badge for pkg.go.dev [\#51](https://github.com/k0kubun/pp/pull/51) ([zakuro9715](https://github.com/zakuro9715))
- Fix typo in README.md [\#50](https://github.com/k0kubun/pp/pull/50) ([bl-ue](https://github.com/bl-ue))
- Add Struct Tag Support for Printing [\#49](https://github.com/k0kubun/pp/pull/49) ([rickbau5](https://github.com/rickbau5))
- Ensure time.Time is from standard library [\#48](https://github.com/k0kubun/pp/pull/48) ([bquenin](https://github.com/bquenin))

## [v3.0.7](https://github.com/k0kubun/pp/tree/v3.0.7) (2020-11-18)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.6...v3.0.7)

**Closed issues:**

- Sort map by key before printing [\#23](https://github.com/k0kubun/pp/issues/23)

**Merged pull requests:**

- Resurrect Go 1.11 support [\#46](https://github.com/k0kubun/pp/pull/46) ([k0kubun](https://github.com/k0kubun))

## [v3.0.6](https://github.com/k0kubun/pp/tree/v3.0.6) (2020-11-14)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.5...v3.0.6)

**Merged pull requests:**

- Sort keys of maps [\#45](https://github.com/k0kubun/pp/pull/45) ([k0kubun](https://github.com/k0kubun))

## [v3.0.5](https://github.com/k0kubun/pp/tree/v3.0.5) (2020-11-14)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.4...v3.0.5)

## [v3.0.4](https://github.com/k0kubun/pp/tree/v3.0.4) (2020-11-12)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.3...v3.0.4)

**Closed issues:**

- Feature request: Pretty print big.Int and big.Float? [\#41](https://github.com/k0kubun/pp/issues/41)

**Merged pull requests:**

- chore: optimize CI [\#44](https://github.com/k0kubun/pp/pull/44) ([deliangyang](https://github.com/deliangyang))
- refactor: use defer release lock [\#43](https://github.com/k0kubun/pp/pull/43) ([deliangyang](https://github.com/deliangyang))

## [v3.0.3](https://github.com/k0kubun/pp/tree/v3.0.3) (2020-08-11)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.2...v3.0.3)

**Closed issues:**

- go mod doesn't install 3.0.2 [\#40](https://github.com/k0kubun/pp/issues/40)

## [v3.0.2](https://github.com/k0kubun/pp/tree/v3.0.2) (2020-05-05)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.1...v3.0.2)

**Closed issues:**

- WithLineInfo print the wrong line  [\#39](https://github.com/k0kubun/pp/issues/39)
- Display of nil slice as same as an empty slice [\#36](https://github.com/k0kubun/pp/issues/36)
- Unsure why number coming out in base 16 [\#27](https://github.com/k0kubun/pp/issues/27)
- disable colors when not output is not a tty [\#26](https://github.com/k0kubun/pp/issues/26)

**Merged pull requests:**

- Allow changing coloringEnabled per pp instance [\#37](https://github.com/k0kubun/pp/pull/37) ([k0kubun](https://github.com/k0kubun))
- support Go modules [\#35](https://github.com/k0kubun/pp/pull/35) ([itchyny](https://github.com/itchyny))
- Add max depth var [\#34](https://github.com/k0kubun/pp/pull/34) ([sumerc](https://github.com/sumerc))
- Allow own instances of pp [\#33](https://github.com/k0kubun/pp/pull/33) ([Eun](https://github.com/Eun))
- fix typo of foreground [\#32](https://github.com/k0kubun/pp/pull/32) ([shogo82148](https://github.com/shogo82148))

## [v3.0.1](https://github.com/k0kubun/pp/tree/v3.0.1) (2019-04-02)

[Full Changelog](https://github.com/k0kubun/pp/compare/v3.0.0...v3.0.1)

## [v3.0.0](https://github.com/k0kubun/pp/tree/v3.0.0) (2019-03-04)

[Full Changelog](https://github.com/k0kubun/pp/compare/v2.4.0...v3.0.0)

## [v2.4.0](https://github.com/k0kubun/pp/tree/v2.4.0) (2019-03-03)

[Full Changelog](https://github.com/k0kubun/pp/compare/v2.3.0...v2.4.0)

**Merged pull requests:**

- Fix newline of map type [\#29](https://github.com/k0kubun/pp/pull/29) ([itchyny](https://github.com/itchyny))
- add MIT license file [\#28](https://github.com/k0kubun/pp/pull/28) ([alteholz](https://github.com/alteholz))
- Update the map printer to properly print maps. [\#25](https://github.com/k0kubun/pp/pull/25) ([denniszl](https://github.com/denniszl))

## [v2.3.0](https://github.com/k0kubun/pp/tree/v2.3.0) (2017-01-23)

[Full Changelog](https://github.com/k0kubun/pp/compare/v2.2.0...v2.3.0)

**Merged pull requests:**

- Add WithLineInfo method for print filename and line number along [\#24](https://github.com/k0kubun/pp/pull/24) ([huydx](https://github.com/huydx))

## [v2.2.0](https://github.com/k0kubun/pp/tree/v2.2.0) (2015-07-23)

[Full Changelog](https://github.com/k0kubun/pp/compare/v2.1.0...v2.2.0)

**Closed issues:**

- please do not use unsafe package [\#20](https://github.com/k0kubun/pp/issues/20)

**Merged pull requests:**

- check whether reflect.Value can call `Interface()` [\#19](https://github.com/k0kubun/pp/pull/19) ([skatsuta](https://github.com/skatsuta))
- Fix indent for slices [\#18](https://github.com/k0kubun/pp/pull/18) ([sdidyk](https://github.com/sdidyk))

## [v2.1.0](https://github.com/k0kubun/pp/tree/v2.1.0) (2015-04-25)

[Full Changelog](https://github.com/k0kubun/pp/compare/v2.0.1...v2.1.0)

**Merged pull requests:**

- Custom colors [\#17](https://github.com/k0kubun/pp/pull/17) ([sdidyk](https://github.com/sdidyk))
- Some changes of printer [\#16](https://github.com/k0kubun/pp/pull/16) ([sdidyk](https://github.com/sdidyk))
- Suppress panic caused by Float values [\#15](https://github.com/k0kubun/pp/pull/15) ([yudai](https://github.com/yudai))

## [v2.0.1](https://github.com/k0kubun/pp/tree/v2.0.1) (2015-03-01)

[Full Changelog](https://github.com/k0kubun/pp/compare/v2.0.0...v2.0.1)

**Merged pull requests:**

- escape sequences to pipe [\#13](https://github.com/k0kubun/pp/pull/13) ([mattn](https://github.com/mattn))

## [v2.0.0](https://github.com/k0kubun/pp/tree/v2.0.0) (2015-02-14)

[Full Changelog](https://github.com/k0kubun/pp/compare/v1.3.0...v2.0.0)

**Closed issues:**

- Fold large buffers [\#8](https://github.com/k0kubun/pp/issues/8)

**Merged pull requests:**

- Fold a large buffer [\#12](https://github.com/k0kubun/pp/pull/12) ([k0kubun](https://github.com/k0kubun))

## [v1.3.0](https://github.com/k0kubun/pp/tree/v1.3.0) (2015-02-14)

[Full Changelog](https://github.com/k0kubun/pp/compare/v1.2.0...v1.3.0)

**Closed issues:**

- time.Time formatter [\#2](https://github.com/k0kubun/pp/issues/2)

**Merged pull requests:**

- Implement time.Time pretty printer [\#11](https://github.com/k0kubun/pp/pull/11) ([k0kubun](https://github.com/k0kubun))

## [v1.2.0](https://github.com/k0kubun/pp/tree/v1.2.0) (2015-02-14)

[Full Changelog](https://github.com/k0kubun/pp/compare/v1.1.0...v1.2.0)

**Merged pull requests:**

- Color escaped characters inside strings [\#10](https://github.com/k0kubun/pp/pull/10) ([motemen](https://github.com/motemen))

## [v1.1.0](https://github.com/k0kubun/pp/tree/v1.1.0) (2015-02-14)

[Full Changelog](https://github.com/k0kubun/pp/compare/v1.0.0...v1.1.0)

**Merged pull requests:**

- Handle circular structures [\#9](https://github.com/k0kubun/pp/pull/9) ([motemen](https://github.com/motemen))

## [v1.0.0](https://github.com/k0kubun/pp/tree/v1.0.0) (2015-01-09)

[Full Changelog](https://github.com/k0kubun/pp/compare/v0.0.1...v1.0.0)

**Closed issues:**

- test failed if Golang over 1.4 [\#5](https://github.com/k0kubun/pp/issues/5)

**Merged pull requests:**

- remove unused struct. [\#7](https://github.com/k0kubun/pp/pull/7) ([walf443](https://github.com/walf443))
- customizable Print\* functions output [\#6](https://github.com/k0kubun/pp/pull/6) ([walf443](https://github.com/walf443))

## [v0.0.1](https://github.com/k0kubun/pp/tree/v0.0.1) (2014-12-29)

[Full Changelog](https://github.com/k0kubun/pp/compare/71948a64abfb9f3877ee472dba16472ca6d8e773...v0.0.1)

**Merged pull requests:**

- fix: `Fprintln` infinite loop bug. [\#3](https://github.com/k0kubun/pp/pull/3) ([kyokomi](https://github.com/kyokomi))
- Support windows [\#1](https://github.com/k0kubun/pp/pull/1) ([k0kubun](https://github.com/k0kubun))



\* *This Changelog was automatically generated by [github_changelog_generator](https://github.com/github-changelog-generator/github-changelog-generator)*
