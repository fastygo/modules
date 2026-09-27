package contentmarkdown

import (
	"testing"
	"testing/fstest"
)

func TestNewLibraryRendersAndFallsBack(t *testing.T) {
	fsys := fstest.MapFS{
		"i18n/en/quickstart.md":    &fstest.MapFile{Data: []byte("# Hello\n\nworld")},
		"i18n/ru/quickstart.md":    &fstest.MapFile{Data: []byte("# Привет\n\nмир")},
		"i18n/en/configuration.md": &fstest.MapFile{Data: []byte("Configuration")},
	}

	lib, err := NewLibrary(LibraryOptions{
		FS:            fsys,
		Pages:         []PageMeta{{Slug: "quickstart", Title: "Quick Start"}, {Slug: "configuration", Title: "Configuration"}},
		Locales:       []string{"en", "ru"},
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	en, ok := lib.Page("en", "quickstart")
	if !ok || en.HTML == "" {
		t.Fatalf("expected EN quickstart, got %+v ok=%v", en, ok)
	}

	ru, ok := lib.Page("ru", "quickstart")
	if !ok || ru.HTML == "" {
		t.Fatalf("expected RU quickstart, got %+v ok=%v", ru, ok)
	}

	if en.HTML == ru.HTML {
		t.Fatalf("expected different HTML for en vs ru")
	}

	fallback, ok := lib.Page("ru", "configuration")
	if !ok || fallback.HTML == "" {
		t.Fatalf("expected fallback content, got %+v ok=%v", fallback, ok)
	}

	if _, ok := lib.Page("en", "missing"); ok {
		t.Fatalf("expected missing page lookup to fail")
	}
}
