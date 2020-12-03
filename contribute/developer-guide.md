# Developer Guide

This guide helps you get started developing Pyroscope.

## Dependencies

Make sure you have the following dependencies installed before setting up your developer environment:

* `git`
* `go`
* `node` + `yarn`
* `rust` (you'll only need it to work on the `agent` part of the code)

### macOS

On macOS we recommend you use [homebrew](https://brew.sh/) to manage dependencies:

```shell
brew install git
brew install go
brew install node
brew install rust

npm install -g yarn
```

## Text Editors

### VS Code

We use `revive` for linting. Add `--config=${workspaceFolder}/revive.toml` to `Go: Lint Flags` section in VS Code settings.

#### Go

If you're using VS Code we would recommend the official [Go extension from Google](https://marketplace.visualstudio.com/items?itemName=golang.Go).


