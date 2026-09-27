// Package view contains data types reused across site shells (theme toggles,
// language toggles, layout metadata).
//
// This module does not render HTML and does not import the render module.
// Each application owns its templ files and decides how to render these values.
package view

import "github.com/fastygo/framework/pkg/app"

// ThemeToggleData drives the dark/light theme switcher rendered in a layout.
type ThemeToggleData struct {
	// Label is the visible label of the toggle (e.g. "Theme").
	Label string
	// SwitchToDarkLabel is the announcement shown when the next click
	// switches the theme to dark.
	SwitchToDarkLabel string
	// SwitchToLightLabel is the equivalent for switching to light.
	SwitchToLightLabel string
}

// LanguageToggleData drives a typical language switcher button.
type LanguageToggleData struct {
	// Label is the visible label of the toggle (e.g. "Language").
	Label string
	// CurrentLocale is the active locale code, e.g. "en".
	CurrentLocale string
	// CurrentLabel is the short display label for CurrentLocale (e.g. "EN").
	CurrentLabel string
	// NextLocale is the locale a click on the toggle switches to.
	NextLocale string
	// NextLabel is the short display label for NextLocale.
	NextLabel string
	// DefaultLocale is the application's fallback locale, surfaced so
	// templates can mark it visually.
	DefaultLocale string
	// AvailableLocales is the closed set of locale codes the toggle
	// can cycle through.
	AvailableLocales []string
	// NextHref is the relative URL (path + query) to navigate to when switching
	// to NextLocale (e.g. "/?lang=ru"). When set, templates may render a plain
	// link so locale changes work without client-side JS.
	NextHref      string
	EnhanceWithJS bool
	SPATarget     string
}

// LayoutData is a common envelope assembled by a feature handler and passed
// to its templ layout component. Examples are free to embed it in a richer
// struct that adds page-specific fields.
type LayoutData struct {
	// Title is the <title> string of the page.
	Title string
	// Locale is the active locale (matches LanguageToggle.CurrentLocale).
	Locale string
	// Active is the slug or path identifying the active nav entry, used
	// by templates to render the current item differently.
	Active string
	// BrandName is the application's display name shown in the header.
	BrandName string
	// NavItems is the primary navigation, typically rendered in a sidebar.
	NavItems []app.NavItem
	// HeaderNavItems is the secondary navigation rendered in the top bar.
	HeaderNavItems []app.NavItem
	// ThemeToggle contains the data for the theme switcher control.
	ThemeToggle ThemeToggleData
	// LanguageToggle contains the data for the language switcher control.
	LanguageToggle LanguageToggleData
}
