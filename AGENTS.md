# Agent notes

This repository is a registry of reusable modules. It is not the framework process.

Each directory under the root is one module with its own tag: `render/vX.Y.Z`, `markdown/vX.Y.Z`, `view/vX.Y.Z`. There is no root `go.mod`.

## Rules

1. Pin `github.com/fastygo/framework` to a published tag. No local `replace`.
2. Do not merge `render` and `view`. View data is not HTML, and the templ files stay in the application.
3. Do not move goldmark into `render`, or templ into `markdown`.
4. Do not add the templ CLI `tool` directive to `render`. Applications that generate templates own that line, at the same `github.com/a-h/templ` version the library requires.
5. Comments and documentation are written in English.

Run before completion, in each module, with `GOWORK=off`:

```text
go test ./...
go vet ./...
gofmt -w .
```
