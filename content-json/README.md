# content-json

`github.com/fastygo/modules/content-json` loads a closed, read-only content
snapshot from an `fs.FS`. The files use the public
[`github.com/fastygo/codex`](https://github.com/fastygo/codex) v0.4 manifest and
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
- the record key `(id, locale)` is duplicated;
- a parent or declared relation points outside the snapshot or to the wrong
  kind.

`Library` returns detached copies from `Manifest`, `Get`, `GetLocale`,
`Entries`, and `ByKind`, so callers cannot mutate the loaded snapshot.

The default paths are `manifest.json` and `entries`. Set `ManifestPath` or
`EntriesDir` when the embedded layout is different. Set `Locale` to change the
default BCP 47 tag used by `Get` (otherwise `en`).

Canonical entry bytes use `canonical.Canonicalize` (`codex-entry/v1`).
`ManifestDigest` is `Manifest.DigestV2` (`codex-manifest/v2`). Neither path
uses `encoding/json` for the digest payload.

## Migrating from content-json v0.1 / Codex v0.3

Pin `github.com/fastygo/codex v0.4.0`. Existing v0.3 entry JSON on disk is
still loaded: `Load` and `DecodeEntry` convert it with `migrate.EntryJSON`.
Each locale in a v0.3 document becomes its own v0.4 record.

Callers that used Codex types directly must follow the Codex v0.4 notes:

- Chrome (`title`, `slug`, `content`, `excerpt`) is a plain string plus
  `locale`. `LocalizedText` and `Entry.Locales` are gone.
- The record key is `(id, locale)`. `Get(id)` picks a locale; use
  `GetLocale(id, locale)` for an exact translation.
- `ManifestDigest` is now `codex-manifest/v2`, not `codex-manifest/v1`.
- Integer ids are rewritten to `<kind>-<n>`. On kind `media`,
  `metadata.post` becomes `metadata.attached_to` (`<kind>-<n>` parent id).
