package contentjson

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fastygo/codex/canonical"
	"github.com/fastygo/codex/content"
)

const manifestJSON = `{
  "name": "site",
  "version": "1",
  "resources": [
    {
      "record": {
        "id": "page",
        "label": "Pages",
        "scope": "tenant",
        "fields": [
          {
            "id": "content",
            "label": "Content",
            "type": "markdown",
            "localized": true
          }
        ]
      }
    }
  ]
}`

// homeEntryJSON is a Codex v0.3 document. Load must accept it via migrate.
const homeEntryJSON = `{
  "id": "home",
  "kind": "page",
  "status": "published",
  "visibility": "public",
  "slug": {
    "en": "home"
  },
  "title": {
    "en": "Home"
  },
  "content": {
    "en": "Hello"
  },
  "locales": {
    "en": {
      "data": {
        "content": "Hello"
      },
      "status": "published",
      "updated_at": "2026-09-28T00:00:00Z"
    }
  },
  "version": 1,
  "created_at": "2026-09-28T00:00:00Z",
  "updated_at": "2026-09-28T00:00:00Z",
  "published_at": "2026-09-28T00:00:00Z"
}`

const homeEntryV04JSON = `{
  "id": "home",
  "locale": "en",
  "kind": "page",
  "status": "published",
  "visibility": "public",
  "slug": "home",
  "title": "Home",
  "content": "Hello",
  "version": 1,
  "created_at": "2026-09-28T00:00:00Z",
  "updated_at": "2026-09-28T00:00:00Z",
  "published_at": "2026-09-28T00:00:00Z"
}`

// Official @ui8kit/contract vector "control: post-1/en-US with metadata.price".
const canonicalEntryJSON = `{"id":"post-1","locale":"en-US","kind":"post","status":"published","visibility":"public","slug":"hello","title":"Hello","version":1,"created_at":"2026-10-25T11:09:00Z","updated_at":"2026-10-25T11:09:00Z","metadata":{"price":{"value":1250}}}`

const canonicalEntryDigest = "codex-entry/v1:sha256:d7ebf2eece26fa0a0b6772b2ad8a14e7eb517882946023bfff1fbbaa8224cc4a"

const canonicalManifestJSON = `{
  "name": "canonical",
  "version": "1",
  "resources": [
    {
      "record": {
        "id": "post",
        "label": "Posts",
        "scope": "tenant",
        "fields": [
          {
            "id": "price",
            "label": "Price",
            "type": "number"
          }
        ]
      }
    }
  ]
}`

func TestLoadValidSnapshot(t *testing.T) {
	library, err := Load(Options{FS: fixtureFS(
		map[string]string{"entries/home.json": homeEntryJSON},
	)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(library.ManifestDigest(), canonical.ManifestDigestV2Prefix) {
		t.Fatalf("manifest digest = %q", library.ManifestDigest())
	}
	entry, ok := library.Get("home")
	if !ok {
		t.Fatal("home entry not found")
	}
	if entry.Locale != "en" || entry.Title != "Home" || entry.Slug != "home" {
		t.Fatalf("entry = %#v", entry)
	}
	pages := library.ByKind("page")
	if len(pages) != 1 || pages[0].ID != "home" {
		t.Fatalf("pages = %#v", pages)
	}

	entry.Title = "Changed"
	again, _ := library.Get("home")
	if again.Title != "Home" {
		t.Fatal("Get exposed library state")
	}
	manifest := library.Manifest()
	manifest.Resources[0].Record.Label = "Changed"
	if library.Manifest().Resources[0].Record.Label != "Pages" {
		t.Fatal("Manifest exposed library state")
	}
}

func TestLoadV04RoundTrip(t *testing.T) {
	library, err := Load(Options{FS: fixtureFS(map[string]string{
		"entries/home.json": homeEntryV04JSON,
	})})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := library.GetLocale("home", "en")
	if !ok {
		t.Fatal("home/en not found")
	}
	if entry.Status != content.StatusPublished || entry.Content != "Hello" {
		t.Fatalf("entry = %#v", entry)
	}
	if library.Len() != 1 {
		t.Fatalf("len = %d", library.Len())
	}
}

func TestLoadMigratesV03Locales(t *testing.T) {
	legacy := `{
  "id": "home",
  "kind": "page",
  "status": "published",
  "visibility": "public",
  "slug": {"en": "home", "ru": "glavnaya"},
  "title": {"en": "Home", "ru": "Главная"},
  "content": {"en": "Hello", "ru": "Привет"},
  "locales": {
    "en": {"data": {"content": "Hello"}, "status": "published", "updated_at": "2026-09-28T00:00:00Z"},
    "ru": {"data": {"content": "Привет"}, "status": "published", "updated_at": "2026-09-28T00:00:00Z"}
  },
  "version": 1,
  "created_at": "2026-09-28T00:00:00Z",
  "updated_at": "2026-09-28T00:00:00Z",
  "published_at": "2026-09-28T00:00:00Z"
}`
	library, err := Load(Options{FS: fixtureFS(map[string]string{
		"entries/home.json": legacy,
	})})
	if err != nil {
		t.Fatal(err)
	}
	if got := library.Locales("home"); len(got) != 2 || got[0] != "en" || got[1] != "ru" {
		t.Fatalf("locales = %#v", got)
	}
	en, _ := library.GetLocale("home", "en")
	ru, _ := library.GetLocale("home", "ru")
	if en.Title != "Home" || ru.Title != "Главная" {
		t.Fatalf("en=%#v ru=%#v", en, ru)
	}
	picked, ok := library.Get("home")
	if !ok || picked.Locale != "en" {
		t.Fatalf("Get default = %#v ok=%v", picked, ok)
	}
}

func TestCanonicalDigestMatchesCodex(t *testing.T) {
	want, err := canonical.Digest(canonicalEntryJSON)
	if err != nil {
		t.Fatal(err)
	}
	if want != canonicalEntryDigest {
		t.Fatalf("codex digest = %q", want)
	}
	got, err := CanonicalDigest(canonicalEntryJSON)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("CanonicalDigest = %q, want %q", got, want)
	}
	text, err := canonical.Canonicalize(canonicalEntryJSON)
	if err != nil {
		t.Fatal(err)
	}
	again, err := canonical.Canonicalize(text)
	if err != nil {
		t.Fatal(err)
	}
	if again != text {
		t.Fatal("canonical form is not stable")
	}

	files := fstest.MapFS{
		"manifest.json":       &fstest.MapFile{Data: []byte(canonicalManifestJSON)},
		"entries/post-1.json": &fstest.MapFile{Data: []byte(canonicalEntryJSON)},
	}
	library, err := Load(Options{FS: files})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := library.GetLocale("post-1", "en-US")
	if !ok || entry.Title != "Hello" {
		t.Fatalf("loaded = %#v ok=%v", entry, ok)
	}
	if _, ok := entry.Metadata["price"]; !ok {
		t.Fatal("price metadata missing")
	}
}

func TestLoadPendingMoneyAndMedia(t *testing.T) {
	manifest := `{
	  "name": "catalog",
	  "version": "1",
	  "resources": [
	    {
	      "record": {
	        "id": "post",
	        "label": "Posts",
	        "scope": "tenant"
	      }
	    },
	    {
	      "record": {
	        "id": "product",
	        "label": "Products",
	        "scope": "tenant",
	        "fields": [
	          {
	            "id": "price",
	            "label": "Price",
	            "type": "number",
	            "rules": [{"name": "fastygo.codex/money"}]
	          },
	          {
	            "id": "currency",
	            "label": "Currency",
	            "type": "string"
	          }
	        ]
	      }
	    },
	    {
	      "record": {
	        "id": "media",
	        "label": "Media",
	        "scope": "tenant",
	        "fields": [
	          {
	            "id": "attached_to",
	            "label": "Attached to",
	            "type": "relation"
	          }
	        ],
	        "relations": [
	          {
	            "id": "attached_to",
	            "source": "media",
	            "target": "post",
	            "cardinality": "one-to-one",
	            "policy": {}
	          }
	        ]
	      }
	    }
	  ]
	}`
	post := `{
	  "id": "post-1",
	  "locale": "en",
	  "kind": "post",
	  "status": "pending",
	  "visibility": "public",
	  "slug": "hello",
	  "title": "Hello",
	  "version": 1,
	  "created_at": "2026-09-28T00:00:00Z",
	  "updated_at": "2026-09-28T00:00:00Z"
	}`
	product := `{
	  "id": "product-1",
	  "locale": "en",
	  "kind": "product",
	  "status": "published",
	  "visibility": "public",
	  "slug": "mug",
	  "title": "Mug",
	  "metadata": {
	    "price": {"value": -1250},
	    "currency": {"value": "USD"}
	  },
	  "version": 1,
	  "created_at": "2026-09-28T00:00:00Z",
	  "updated_at": "2026-09-28T00:00:00Z",
	  "published_at": "2026-09-28T00:00:00Z"
	}`
	media := `{
	  "id": "7",
	  "locale": "en",
	  "kind": "media",
	  "status": "published",
	  "visibility": "public",
	  "slug": "photo",
	  "title": "Photo",
	  "metadata": {
	    "post": {"value": 1}
	  },
	  "version": 1,
	  "created_at": "2026-09-28T00:00:00Z",
	  "updated_at": "2026-09-28T00:00:00Z",
	  "published_at": "2026-09-28T00:00:00Z"
	}`
	files := fstest.MapFS{
		"manifest.json":        &fstest.MapFile{Data: []byte(manifest)},
		"entries/post.json":    &fstest.MapFile{Data: []byte(post)},
		"entries/product.json": &fstest.MapFile{Data: []byte(product)},
		"entries/media.json":   &fstest.MapFile{Data: []byte(media)},
	}
	library, err := Load(Options{FS: files})
	if err != nil {
		t.Fatal(err)
	}
	pending, _ := library.GetLocale("post-1", "en")
	if pending.Status != content.StatusPending {
		t.Fatalf("status = %q", pending.Status)
	}
	item, _ := library.Get("product-1")
	if moneyMinorUnits(item.Metadata["price"].Value) != "-1250" {
		t.Fatalf("price = %#v", item.Metadata["price"].Value)
	}
	if item.Metadata["currency"].Value != "USD" {
		t.Fatalf("currency = %#v", item.Metadata["currency"].Value)
	}
	attachment, ok := library.GetLocale("media-7", "en")
	if !ok {
		t.Fatal("media-7 not found")
	}
	if _, hasPost := attachment.Metadata["post"]; hasPost {
		t.Fatal("metadata.post was not rewritten")
	}
	if attachment.Metadata["attached_to"].Value != "post-1" {
		t.Fatalf("attached_to = %#v", attachment.Metadata["attached_to"].Value)
	}
}

func moneyMinorUnits(value any) string {
	switch typed := value.(type) {
	case json.Number:
		return typed.String()
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func TestEntriesAreSorted(t *testing.T) {
	second := strings.ReplaceAll(homeEntryJSON, `"id": "home"`, `"id": "about"`)
	second = strings.ReplaceAll(second, `"en": "home"`, `"en": "about"`)
	second = strings.ReplaceAll(second, `"en": "Home"`, `"en": "About"`)
	library, err := Load(Options{FS: fixtureFS(map[string]string{
		"entries/z-home.json":  homeEntryJSON,
		"entries/a-about.json": second,
	})})
	if err != nil {
		t.Fatal(err)
	}
	entries := library.Entries()
	if len(entries) != 2 || entries[0].ID != "about" || entries[1].ID != "home" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	entry := strings.Replace(homeEntryJSON, `"id": "home",`, `"id": "home", "surprise": true,`, 1)
	_, err := Load(Options{FS: fixtureFS(map[string]string{"entries/home.json": entry})})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsDuplicateID(t *testing.T) {
	_, err := Load(Options{FS: fixtureFS(map[string]string{
		"entries/first.json":  homeEntryJSON,
		"entries/second.json": homeEntryJSON,
	})})
	if err == nil || !strings.Contains(err.Error(), "duplicate entry") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsUndeclaredKind(t *testing.T) {
	entry := strings.Replace(homeEntryJSON, `"kind": "page"`, `"kind": "product"`, 1)
	_, err := Load(Options{FS: fixtureFS(map[string]string{"entries/product.json": entry})})
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsMissingParent(t *testing.T) {
	entry := strings.Replace(homeEntryJSON, `"version": 1,`, `"parent_id": "missing", "version": 1,`, 1)
	_, err := Load(Options{FS: fixtureFS(map[string]string{"entries/home.json": entry})})
	if err == nil || !strings.Contains(err.Error(), "missing parent") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsParentCycle(t *testing.T) {
	home := strings.Replace(homeEntryJSON, `"version": 1,`, `"parent_id": "about", "version": 1,`, 1)
	about := strings.ReplaceAll(homeEntryJSON, `"id": "home"`, `"id": "about"`)
	about = strings.ReplaceAll(about, `"en": "home"`, `"en": "about"`)
	about = strings.ReplaceAll(about, `"en": "Home"`, `"en": "About"`)
	about = strings.Replace(about, `"version": 1,`, `"parent_id": "home", "version": 1,`, 1)
	_, err := Load(Options{FS: fixtureFS(map[string]string{
		"entries/home.json":  home,
		"entries/about.json": about,
	})})
	if err == nil || !strings.Contains(err.Error(), "parent cycle") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsTaxonomyNotAssignedToKind(t *testing.T) {
	entry := strings.Replace(
		homeEntryJSON,
		`"version": 1,`,
		`"terms": [{"taxonomy": "topic", "term_id": "go"}], "version": 1,`,
		1,
	)
	_, err := Load(Options{FS: fixtureFS(map[string]string{"entries/home.json": entry})})
	if err == nil || !strings.Contains(err.Error(), "not assigned") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsDanglingDeclaredRelation(t *testing.T) {
	manifest := `{
	  "name": "relations",
	  "version": "1",
	  "resources": [
	    {
	      "record": {
	        "id": "author",
	        "label": "Authors",
	        "scope": "tenant"
	      }
	    },
	    {
	      "record": {
	        "id": "article",
	        "label": "Articles",
	        "scope": "tenant",
	        "fields": [
	          {
	            "id": "author",
	            "label": "Author",
	            "type": "relation"
	          }
	        ],
	        "relations": [
	          {
	            "id": "author",
	            "source": "article",
	            "target": "author",
	            "cardinality": "one-to-one",
	            "policy": {}
	          }
	        ]
	      }
	    }
	  ]
	}`
	entry := strings.Replace(homeEntryJSON, `"kind": "page"`, `"kind": "article"`, 1)
	entry = strings.Replace(entry, `"locales": {`, `"metadata": {"author": {"value": "missing-author"}}, "locales": {`, 1)
	files := fstest.MapFS{
		"manifest.json":        &fstest.MapFile{Data: []byte(manifest)},
		"entries/article.json": &fstest.MapFile{Data: []byte(entry)},
	}
	_, err := Load(Options{FS: files})
	if err == nil || !strings.Contains(err.Error(), "missing entry") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeEntryUsesJSONNumbers(t *testing.T) {
	entry, err := DecodeEntry(strings.NewReader(homeEntryJSON))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Version != 1 || entry.Status != content.StatusPublished || entry.Locale != "en" {
		t.Fatalf("entry = %#v", entry)
	}
	v04, err := DecodeEntry(strings.NewReader(homeEntryV04JSON))
	if err != nil {
		t.Fatal(err)
	}
	if v04.Title != "Home" || v04.Locale != "en" {
		t.Fatalf("v0.4 entry = %#v", v04)
	}
}

func fixtureFS(entries map[string]string) fstest.MapFS {
	files := fstest.MapFS{
		"manifest.json": &fstest.MapFile{Data: []byte(manifestJSON)},
	}
	for name, data := range entries {
		files[name] = &fstest.MapFile{Data: []byte(data)}
	}
	return files
}
