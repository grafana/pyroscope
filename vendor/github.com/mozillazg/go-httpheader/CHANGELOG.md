# Changelog

## [0.3.1] (2022-04-09)

* fix Decode: don't fill value for struct fields that don't exist in header


## [0.3.0] (2021-01-24)

* add `func Decode(header http.Header, v interface{}) error` to support decoding headers into struct

## [0.2.1] (2018-11-03)

* add go.mod file to identify as a module


## [0.2.0] (2017-06-24)

* support http.Header field.


## 0.1.0 (2017-06-10)

* Initial Release

[0.2.0]: https://github.com/mozillazg/go-httpheader/compare/v0.1.0...v0.2.0
[0.2.1]: https://github.com/mozillazg/go-httpheader/compare/v0.2.0...v0.2.1
[0.3.0]: https://github.com/mozillazg/go-httpheader/compare/v0.2.1...v0.3.0
