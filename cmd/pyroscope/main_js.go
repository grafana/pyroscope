package main

import (
	"fmt"
	"syscall/js"
)

func print(msg string) {
	fmt.Println(msg)
}

func main() {
	exports := map[string]interface{}{}
	exports["print"] = jsFunc(func(args []js.Value) interface{} {
		print(args[0].String())
		return nil
	})
	js.Global().Set("wasm", js.ValueOf(exports))
	select {}
}

func jsFunc(f func([]js.Value) interface{}) js.Value {
	return js.ValueOf(js.FuncOf(func(this js.Value, args []js.Value) (ret interface{}) {
		defer func() {
			r := recover()
			if r != nil {
				ret = fmt.Sprint(r)
			}
		}()
		return f(args)
	}))
}
