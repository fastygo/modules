package contentjson

import (
	"strings"
	"testing"
	"testing/fstest"

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

func TestLoadValidSnapshot(t *testing.T) {
	library, err := Load(Options{FS: fixtureFS(
		map[string]string{"entries/home.json": homeEntryJSON},
	)})
	if err != nil {
		t.Fatal(err)
	}
	if library.ManifestDigest() == "" {
		t.Fatal("manifest digest is empty")
	}
	entry, ok := library.Get("home")
	if !ok {
		t.Fatal("home entry not found")
	}
	if entry.Title.Value("en", "") != "Home" {
		t.Fatalf("title = %#v", entry.Title)
	}
	pages := library.ByKind("page")
	if len(pages) != 1 || pages[0].ID != "home" {
		t.Fatalf("pages = %#v", pages)
	}

	// Reads are detached from the immutable library.
	entry.Title["en"] = "Changed"
	again, _ := library.Get("home")
	if again.Title["en"] != "Home" {
		t.Fatal("Get exposed library state")
	}
	manifest := library.Manifest()
	manifest.Resources[0].Record.Label = "Changed"
	if library.Manifest().Resources[0].Record.Label != "Pages" {
		t.Fatal("Manifest exposed library state")
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
	if err == nil || !strings.Contains(err.Error(), "duplicate entry id") {
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
	if entry.Version != 1 || entry.Status != content.StatusPublished {
		t.Fatalf("entry = %#v", entry)
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
