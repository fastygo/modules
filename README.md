# Go ModulY

Reusable modules that sit next to [`github.com/fastygo/framework`](https://github.com/fastygo/framework). Each directory is its own module. Import the tag. There is no root module and no local `replace`.

Current shared pins are Framework **v0.4.0** and Codex **v0.3.0**.

| Module | Import | Tag |
| --- | --- | --- |
| Read-only Codex JSON fixtures | `github.com/fastygo/modules/content-json` | `content-json/v0.1.0` |
| Templ response writer | `github.com/fastygo/modules/render` | `render/v0.1.0` |
| Markdown pages | `github.com/fastygo/modules/markdown` | `markdown/v0.1.0` |
| Theme and language data | `github.com/fastygo/modules/view` | `view/v0.1.0` |

```bash
go get github.com/fastygo/modules/content-json@v0.1.0
go get github.com/fastygo/modules/render@v0.1.0
go get github.com/fastygo/modules/markdown@v0.1.0
go get github.com/fastygo/modules/view@v0.1.0
```

`content-json` strictly loads a Codex manifest and entries from `fs.FS`.
Static and embedded content therefore uses the same
`github.com/fastygo/codex` contract as a remote GoBackend deployment. The
module validates the whole snapshot before exposing immutable reads; it does
not define product schemas or require a backend process.

`render` and `view` stay separate. `render` writes a templ component to an HTTP response and depends on `pkg/cache`. `view` only builds data for a theme or language control and depends on `pkg/app` and `pkg/web/locale`. It does not render HTML. The templ files for those controls stay in the application.

`markdown` stays separate so a program that only renders templ does not take goldmark, and a program that only compiles markdown does not take templ.

## templ CLI

`render` requires library `github.com/a-h/templ v0.3.1001`. The CLI is the same module. An application that runs `templ generate` pins that CLI with a tool directive, which needs Go 1.24 or newer:

```bash
go get github.com/a-h/templ@v0.3.1001
go get -tool github.com/a-h/templ/cmd/templ@v0.3.1001
go tool templ generate ./...
```

The tool line belongs in the application, not in `render`. Putting it in the library would add the CLI's own dependencies to every importer.

Bun is not a Go module. Record it in `package.json` as `"packageManager": "bun@1.3.x"`, or check the version in the application's doctor command.

## Check

From this repository, with the workspace turned off:

```bash
GOWORK=off go test ./...
GOWORK=off go vet ./...
```

Run that in `content-json`, `render`, `markdown`, and `view`.
