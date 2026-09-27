// Package contentmarkdown offers a tiny content-rendering pipeline for
// static markdown sources embedded into the binary at compile-time.
//
// It is intentionally minimal: a content registry (slug + title), a render
// pipeline that converts markdown to HTML once at startup, and a Lookup API
// returning a ready-to-serve PageRender keyed by locale and slug.
//
// This package is its own module, github.com/fastygo/modules/markdown,
// so the framework root does not pull in github.com/yuin/goldmark.
// The package identifier is contentmarkdown so user code typically aliases it:
//
//	import md "github.com/fastygo/modules/markdown"
package contentmarkdown

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/yuin/goldmark"

	"github.com/fastygo/framework/pkg/web/locale"
)

// PageMeta describes a single content page.
type PageMeta struct {
	// Slug is the URL-safe identifier used both as the source filename
	// (i18n/<locale>/<slug>.md) and as the lookup key in Library.Page.
	Slug string
	// Title is the human-readable page title; the library does not use
	// it internally — it is surfaced to handlers verbatim.
	Title string
}

// PageRender is the rendered version of a PageMeta for a given locale.
type PageRender struct {
	// Slug mirrors PageMeta.Slug for convenience when rendering nav menus.
	Slug string
	// Title mirrors PageMeta.Title for the same reason.
	Title string
	// HTML is the goldmark-rendered output. Treat it as already-safe
	// HTML if your markdown sources are trusted (the framework's blog
	// and docs examples render it via templ.Raw).
	HTML string
}

// Renderer converts markdown bytes to HTML bytes.
type Renderer interface {
	Convert(source []byte, dst *bytes.Buffer) error
}

type goldmarkRenderer struct {
	md goldmark.Markdown
}

func (g goldmarkRenderer) Convert(source []byte, dst *bytes.Buffer) error {
	return g.md.Convert(source, dst)
}

// DefaultRenderer returns a Renderer backed by goldmark with the default
// extensions.
func DefaultRenderer() Renderer {
	return goldmarkRenderer{md: goldmark.New()}
}

// Library is a thread-safe in-memory registry of rendered pages.
//
// Build it once at startup with NewLibrary, then call Page(locale, slug) at
// request time. The library never touches the filesystem after construction
// (all markdown is read and converted eagerly).
type Library struct {
	pages         []PageMeta
	defaultLocale string
	locales       []string

	mu      sync.RWMutex
	storage map[string]map[string]PageRender // locale -> slug -> render
}

// LibraryOptions configures library construction.
type LibraryOptions struct {
	// FS is the file system that contains the markdown sources.
	FS fs.FS
	// Pages enumerates the pages to load. Each page is required to exist for
	// the default locale and is optional for the rest.
	Pages []PageMeta
	// Locales is the list of locales to render content for.
	Locales []string
	// DefaultLocale is the fallback locale used when a translation is missing.
	DefaultLocale string
	// PathTemplate optionally overrides the path template used to look up
	// markdown files. The default value is "i18n/{locale}/{slug}.md".
	// The placeholders {locale} and {slug} are substituted at lookup time.
	PathTemplate string
	// Renderer overrides the default markdown renderer.
	Renderer Renderer
}

// NewLibrary constructs and pre-renders a Library.
func NewLibrary(opts LibraryOptions) (*Library, error) {
	if opts.FS == nil {
		return nil, fmt.Errorf("contentmarkdown: FS is required")
	}
	if len(opts.Pages) == 0 {
		return nil, fmt.Errorf("contentmarkdown: at least one page is required")
	}

	locales := locale.Normalize(opts.Locales...)
	if len(locales) == 0 {
		locales = []string{"en"}
	}

	defaultLocale := strings.ToLower(strings.TrimSpace(opts.DefaultLocale))
	if defaultLocale == "" || !locale.Contains(locales, defaultLocale) {
		defaultLocale = locales[0]
	}

	pathTemplate := opts.PathTemplate
	if pathTemplate == "" {
		pathTemplate = "i18n/{locale}/{slug}.md"
	}

	renderer := opts.Renderer
	if renderer == nil {
		renderer = DefaultRenderer()
	}

	storage := make(map[string]map[string]PageRender, len(locales))

	for _, page := range opts.Pages {
		defaultPath := resolvePath(pathTemplate, defaultLocale, page.Slug)
		defaultMD, err := fs.ReadFile(opts.FS, defaultPath)
		if err != nil {
			return nil, fmt.Errorf("contentmarkdown: read %s: %w", defaultPath, err)
		}

		for _, loc := range locales {
			source := defaultMD
			if loc != defaultLocale {
				if md, err := fs.ReadFile(opts.FS, resolvePath(pathTemplate, loc, page.Slug)); err == nil {
					source = md
				}
			}

			var html bytes.Buffer
			if err := renderer.Convert(source, &html); err != nil {
				return nil, fmt.Errorf("contentmarkdown: render %s/%s: %w", loc, page.Slug, err)
			}

			localeStore, ok := storage[loc]
			if !ok {
				localeStore = make(map[string]PageRender, len(opts.Pages))
				storage[loc] = localeStore
			}
			localeStore[page.Slug] = PageRender{
				Slug:  page.Slug,
				Title: page.Title,
				HTML:  html.String(),
			}
		}
	}

	pages := make([]PageMeta, len(opts.Pages))
	copy(pages, opts.Pages)

	return &Library{
		pages:         pages,
		defaultLocale: defaultLocale,
		locales:       locales,
		storage:       storage,
	}, nil
}

// Pages returns the static metadata list (sorted by Slug for stability).
func (l *Library) Pages() []PageMeta {
	out := make([]PageMeta, len(l.pages))
	copy(out, l.pages)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Locales returns the locales the library was built with.
func (l *Library) Locales() []string {
	out := make([]string, len(l.locales))
	copy(out, l.locales)
	return out
}

// DefaultLocale exposes the configured default locale.
func (l *Library) DefaultLocale() string { return l.defaultLocale }

// Page returns the rendered page for the requested locale, falling back to
// the default locale when the locale itself is unknown but the slug exists.
// It returns ok=false when the slug is unknown.
func (l *Library) Page(loc string, slug string) (PageRender, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if localeStore, ok := l.storage[loc]; ok {
		if rendered, ok := localeStore[slug]; ok {
			return rendered, true
		}
	}
	if loc != l.defaultLocale {
		if localeStore, ok := l.storage[l.defaultLocale]; ok {
			if rendered, ok := localeStore[slug]; ok {
				return rendered, true
			}
		}
	}
	return PageRender{}, false
}

func resolvePath(template, loc, slug string) string {
	out := strings.ReplaceAll(template, "{locale}", loc)
	out = strings.ReplaceAll(out, "{slug}", slug)
	return out
}
