# content-json

`github.com/fastygo/modules/content-json` loads a closed, read-only content
snapshot from an `fs.FS`. The files use the public
[`github.com/fastygo/codex`](https://github.com/fastygo/codex) manifest and
entry JSON contracts.

This module does not define another content model and does not connect to
GoBackend. A static site can embed the same shapes that a remote Codex adapter
returns:

```text
content/
├── manifest.json
└── entries/
    ├── home.json
    └── about.json
```

```go
package site

import (
	"embed"
	"io/fs"

	contentjson "github.com/fastygo/modules/content-json"
)

//go:embed content
var files embed.FS

func loadContent() (*contentjson.Library, error) {
	root, err := fs.Sub(files, "content")
	if err != nil {
		return nil, err
	}
	return contentjson.Load(contentjson.Options{FS: root})
}
```

Loading is atomic. It fails when:

- JSON contains an unknown, redundant, or unstable field;
- the manifest or an entry violates Codex validation;
- an entry kind is absent from the manifest;
- identifiers are duplicated;
- a parent or declared relation points outside the snapshot or to the wrong
  kind.

`Library` returns detached copies from `Manifest`, `Get`, `Entries`, and
`ByKind`, so callers cannot mutate the loaded snapshot.

The default paths are `manifest.json` and `entries`. Set `ManifestPath` or
`EntriesDir` when the embedded layout is different.
