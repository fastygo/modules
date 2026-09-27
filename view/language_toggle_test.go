package view

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fastygo/framework/pkg/web/locale"
)

func TestBuildLanguageToggleAutoNext(t *testing.T) {
	got := BuildLanguageToggle(LanguageToggleConfig{
		CurrentLocale: "en",
		DefaultLocale: "en",
		Available:     []string{"en", "ru", "de"},
		LocaleLabels:  map[string]string{"en": "EN", "ru": "RU", "de": "DE"},
	})
	if got.NextLocale != "ru" {
		t.Fatalf("expected next locale ru, got %q", got.NextLocale)
	}
	if got.CurrentLabel != "EN" {
		t.Fatalf("expected current label EN, got %q", got.CurrentLabel)
	}
	if got.NextLabel != "RU" {
		t.Fatalf("expected next label RU, got %q", got.NextLabel)
	}
}

func TestBuildLanguageToggleSingleLocale(t *testing.T) {
	got := BuildLanguageToggle(LanguageToggleConfig{
		CurrentLocale: "en",
		Available:     []string{"en"},
		LocaleLabels:  map[string]string{"en": "EN"},
	})
	if got.NextLocale != "" {
		t.Fatalf("expected empty next locale, got %q", got.NextLocale)
	}
	if got.CurrentLabel != "EN" {
		t.Fatalf("expected current label EN, got %q", got.CurrentLabel)
	}
}

func TestBuildLanguageToggleExplicitNext(t *testing.T) {
	got := BuildLanguageToggle(LanguageToggleConfig{
		CurrentLocale: "en",
		Available:     []string{"en", "ru"},
		NextLocale:    "ru",
		NextLabel:     "Russian",
	})
	if got.NextLocale != "ru" {
		t.Fatalf("expected ru, got %q", got.NextLocale)
	}
	if got.NextLabel != "Russian" {
		t.Fatalf("expected explicit label, got %q", got.NextLabel)
	}
}

func TestBuildLanguageToggleAutoNextHref(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/welcome?foo=1", nil)
	got := BuildLanguageToggle(LanguageToggleConfig{
		CurrentLocale: "en",
		DefaultLocale: "en",
		Available:     []string{"en", "ru", "de"},
		Request:       req,
	})
	if got.NextHref != "/welcome?foo=1&lang=ru" {
		t.Fatalf("expected next href /welcome?foo=1&lang=ru, got %q", got.NextHref)
	}
}

func TestBuildLanguageToggleFromContextQuery(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := BuildLanguageToggleFromContext(
			r.Context(),
			WithLocaleLabels(map[string]string{"en": "EN", "ru": "RU"}),
		)
		if got.NextHref != "/welcome?lang=ru" {
			t.Fatalf("expected next href /welcome?lang=ru, got %q", got.NextHref)
		}
		if got.EnhanceWithJS {
			t.Fatalf("expected SPA enhancement disabled by default")
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/welcome", nil)
	req.AddCookie(&http.Cookie{
		Name:  "lang",
		Value: "en",
	})
	Middleware := locale.MiddlewareWithSPAMode(&locale.QueryStrategy{
		Available: []string{"en", "ru"},
		Default:   "en",
	}, false)
	Middleware(handler).ServeHTTP(httptest.NewRecorder(), req)
}

func TestBuildLanguageToggleFromContextPath(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := BuildLanguageToggleFromContext(
			r.Context(),
			WithLocaleLabels(map[string]string{"en": "EN", "ru": "RU"}),
		)
		if got.CurrentLocale != "ru" {
			t.Fatalf("expected current locale ru, got %q", got.CurrentLocale)
		}
		if got.NextHref != "/en/welcome?foo=1" {
			t.Fatalf("expected next href /en/welcome?foo=1, got %q", got.NextHref)
		}
		if got.NextLocale != "en" {
			t.Fatalf("expected next locale en, got %q", got.NextLocale)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ru/welcome?foo=1", nil)
	Middleware := locale.MiddlewareWithSPAMode(&locale.PathPrefixStrategy{
		Available: []string{"en", "ru"},
		Default:   "en",
	}, true)
	Middleware(handler).ServeHTTP(httptest.NewRecorder(), req)
}

// TestLanguageToggleOptionsApplyToConfig pins the With* helper behaviour so the
// builder API stays stable for downstream templates.
func TestLanguageToggleOptionsApplyToConfig(t *testing.T) {
	cfg := LanguageToggleConfig{}
	opts := []LanguageToggleOption{
		WithLabel("Language"),
		WithCurrentLabel("EN"),
		WithNextLocale("ru"),
		WithNextLabel("RU"),
		WithEnhanceWithJS(true),
		WithSPATarget("  #app  "),
		WithDefaultLocale("en"),
		WithAvailable([]string{"en", "ru", "de"}),
		WithLocaleLabels(map[string]string{"en": "EN", "ru": "RU"}),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.Label != "Language" {
		t.Fatalf("WithLabel: got %q", cfg.Label)
	}
	if cfg.CurrentLabel != "EN" {
		t.Fatalf("WithCurrentLabel: got %q", cfg.CurrentLabel)
	}
	if cfg.NextLocale != "ru" {
		t.Fatalf("WithNextLocale: got %q", cfg.NextLocale)
	}
	if cfg.NextLabel != "RU" {
		t.Fatalf("WithNextLabel: got %q", cfg.NextLabel)
	}
	if !cfg.EnhanceWithJS {
		t.Fatalf("WithEnhanceWithJS: expected true")
	}
	if cfg.SPATarget != "#app" {
		t.Fatalf("WithSPATarget: expected trimmed value, got %q", cfg.SPATarget)
	}
	if cfg.DefaultLocale != "en" {
		t.Fatalf("WithDefaultLocale: got %q", cfg.DefaultLocale)
	}
	if len(cfg.Available) != 3 || cfg.Available[2] != "de" {
		t.Fatalf("WithAvailable: got %#v", cfg.Available)
	}
	if cfg.LocaleLabels["ru"] != "RU" {
		t.Fatalf("WithLocaleLabels: got %#v", cfg.LocaleLabels)
	}
}

// TestNormalizeStringSliceEdgeCases covers the trimming/de-duplication logic so
// regressions surface as failing unit tests rather than rendering quirks.
func TestNormalizeStringSliceEdgeCases(t *testing.T) {
	if got := normalizeStringSlice(nil); got != nil {
		t.Fatalf("nil slice: expected nil, got %#v", got)
	}
	if got := normalizeStringSlice([]string{"  ", "\t"}); len(got) != 0 {
		t.Fatalf("blank entries: expected empty slice, got %#v", got)
	}
	got := normalizeStringSlice([]string{"EN", " en ", "RU", "ru", "", "de"})
	want := []string{"en", "ru", "de"}
	if len(got) != len(want) {
		t.Fatalf("dedupe: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupe[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildLanguageToggleFromContextPathSpaEnabled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := BuildLanguageToggleFromContext(r.Context())
		if !got.EnhanceWithJS {
			t.Fatalf("expected SPA enhancement enabled from context")
		}
		if got.SPATarget != "main" {
			t.Fatalf("expected default SPA target main, got %q", got.SPATarget)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ru/welcome", nil)
	Middleware := locale.MiddlewareWithSPAMode(&locale.PathPrefixStrategy{
		Available: []string{"en", "ru"},
		Default:   "en",
	}, true)
	Middleware(handler).ServeHTTP(httptest.NewRecorder(), req)
}
