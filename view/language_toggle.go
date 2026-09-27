package view

import (
	"context"
	"net/http"
	"strings"

	"github.com/fastygo/framework/pkg/web/locale"
)

// LanguageToggleConfig describes the locale metadata needed to compute a
// LanguageToggleData value at request-time.
type LanguageToggleConfig struct {
	// CurrentLocale is the active locale code (e.g. "en").
	CurrentLocale string
	// DefaultLocale is the application's fallback locale.
	DefaultLocale string
	// Available is the closed set of locales the toggle can cycle through.
	Available []string

	// Label is the visible label of the toggle, e.g. "Language".
	Label string
	// CurrentLabel is the short representation of the current locale (e.g. "EN").
	// When empty, LocaleLabels[CurrentLocale] is used.
	CurrentLabel string
	// NextLocale optionally overrides the auto-detected next locale.
	NextLocale string
	// NextLabel optionally overrides the auto-detected next-locale label.
	NextLabel string
	// Request is an optional request used to pre-compute NextHref.
	//
	// When set, BuildLanguageToggle computes data.NextHref so templates can
	// render a plain link without client-side locale toggling.
	Request *http.Request
	// EnhanceWithJS enables the SPA enhancement path in templates.
	EnhanceWithJS bool
	// SPATarget is the selector for the fragment replaced in SPA mode.
	SPATarget string
	// LocaleLabels maps a locale code to its display label, e.g. "en" -> "EN".
	LocaleLabels map[string]string
}

// LanguageToggleOption customizes a LanguageToggleConfig for BuildLanguageToggleFromContext.
type LanguageToggleOption func(*LanguageToggleConfig)

// WithLocaleLabels injects locale-specific labels shown in the toggle control.
func WithLocaleLabels(labels map[string]string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.LocaleLabels = labels
	}
}

// WithLabel sets the visible toggle label.
func WithLabel(label string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.Label = label
	}
}

// WithCurrentLabel sets the displayed label for the active locale.
func WithCurrentLabel(label string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.CurrentLabel = label
	}
}

// WithNextLocale forces the next locale displayed by the toggle.
func WithNextLocale(locale string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.NextLocale = locale
	}
}

// WithNextLabel sets the label for the selected next locale.
func WithNextLabel(label string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.NextLabel = label
	}
}

// WithEnhanceWithJS enables the SPA enhancement path in templates.
func WithEnhanceWithJS(enabled bool) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.EnhanceWithJS = enabled
	}
}

// WithSPATarget sets the selector for SPA fragment replacement.
func WithSPATarget(selector string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.SPATarget = strings.TrimSpace(selector)
	}
}

// WithDefaultLocale sets the default locale for this toggle.
func WithDefaultLocale(locale string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.DefaultLocale = locale
	}
}

// WithAvailable sets the candidate locales for toggle cycling.
func WithAvailable(values []string) LanguageToggleOption {
	return func(cfg *LanguageToggleConfig) {
		cfg.Available = values
	}
}

// BuildLanguageToggleFromContext reads locale context and builds a LanguageToggleData
// without duplicating locale plumbing in handlers.
func BuildLanguageToggleFromContext(ctx context.Context, opts ...LanguageToggleOption) LanguageToggleData {
	request := locale.RequestFromContext(ctx)
	if request != nil {
		request = request.WithContext(ctx)
	}
	cfg := LanguageToggleConfig{
		CurrentLocale: locale.From(ctx),
		DefaultLocale: locale.Default(ctx),
		Available:     locale.Available(ctx),
		Request:       request,
	}
	for _, option := range opts {
		option(&cfg)
	}
	cfg.EnhanceWithJS = cfg.EnhanceWithJS || locale.SPAMode(ctx)
	if cfg.EnhanceWithJS && cfg.SPATarget == "" {
		cfg.SPATarget = "main"
	}
	return BuildLanguageToggle(cfg)
}

// BuildLanguageToggle materialises a LanguageToggleData from the given config,
// falling back to sensible defaults when individual fields are missing.
//
// In particular it picks the next available locale if NextLocale is empty,
// and derives short labels from LocaleLabels (or upper-cased locale code).
func BuildLanguageToggle(cfg LanguageToggleConfig) LanguageToggleData {
	current := strings.ToLower(strings.TrimSpace(cfg.CurrentLocale))
	available := normalizeStringSlice(cfg.Available)
	if len(available) == 0 && current != "" {
		available = []string{current}
	}

	nextLocale := strings.ToLower(strings.TrimSpace(cfg.NextLocale))
	if !contains(available, nextLocale) || nextLocale == current {
		nextLocale = ""
		for _, candidate := range available {
			if candidate != current {
				nextLocale = candidate
				break
			}
		}
	}

	currentLabel := cfg.CurrentLabel
	if currentLabel == "" {
		currentLabel = labelFor(current, cfg.LocaleLabels)
	}

	nextLabel := cfg.NextLabel
	if nextLabel == "" {
		nextLabel = labelFor(nextLocale, cfg.LocaleLabels)
	}

	return LanguageToggleData{
		Label:            cfg.Label,
		CurrentLocale:    current,
		CurrentLabel:     currentLabel,
		NextLocale:       nextLocale,
		NextLabel:        nextLabel,
		DefaultLocale:    strings.ToLower(strings.TrimSpace(cfg.DefaultLocale)),
		AvailableLocales: available,
		NextHref:         nextHref(cfg.Request, nextLocale, cfg.DefaultLocale),
		EnhanceWithJS:    cfg.EnhanceWithJS,
		SPATarget:        cfg.SPATarget,
	}
}

func nextHref(r *http.Request, nextLocale, defaultLocale string) string {
	if r == nil || nextLocale == "" {
		return ""
	}
	if href := locale.HrefFor(r.Context(), nextLocale); href != "" {
		return href
	}
	return locale.BuildLangHref(r, nextLocale, defaultLocale)
}

func labelFor(locale string, labels map[string]string) string {
	if locale == "" {
		return ""
	}
	if labels != nil {
		if label, ok := labels[locale]; ok && label != "" {
			return label
		}
	}
	return strings.ToUpper(locale)
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		v := strings.ToLower(strings.TrimSpace(raw))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
