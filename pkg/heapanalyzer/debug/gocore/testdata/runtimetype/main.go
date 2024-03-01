package main

import (
	pkga "example.com/m/path-a/pkg"
	pkgb "example.com/m/path-b/pkg"
)

var global []interface{}

func main() {
	global = append(global, pkga.NewIfaceDirect(), pkga.NewIfaceInDirect(), pkgb.NewIfaceDirect(), pkgb.NewIfaceInDirect())

	_ = *(*int)(nil)
}
