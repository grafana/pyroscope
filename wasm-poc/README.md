# WebAssembly Proof of Concept

1. Compile to webassebly (from project root directory):

```
$ make build-wasm
```

2. Launch the server:

```
$ go run server.go
```

3. Go to the browser at http://localhost:3000/wasm.html and check the console, you should see `Hello from go!`
