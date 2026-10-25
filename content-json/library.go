// Package contentjson loads a closed, read-only Codex content set from JSON.
//
// It is a storage adapter, not a second content contract. Manifest and entry
// files use the public github.com/fastygo/codex JSON shapes and are validated
// by Codex before the library becomes available.
package contentjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/fastygo/codex/canonical"
	"github.com/fastygo/codex/content"
	"github.com/fastygo/codex/migrate"
	"github.com/fastygo/codex/schema"
	"github.com/fastygo/codex/validate"
	"github.com/fastygo/codex/validation"
)

const (
	defaultManifestPath = "manifest.json"
	defaultEntriesDir   = "entries"
)

// Options selects fixture paths inside FS. Zero values use manifest.json and
// entries/*.json. Locale is the default BCP 47 tag for Get and for v0.3 files
// that do not name a locale; the zero value is content.DefaultLocale.
type Options struct {
	FS           fs.FS
	ManifestPath string
	EntriesDir   string
	Locale       string
}

type recordKey struct {
	id     content.ID
	locale string
}

// Library is an immutable, in-memory Codex snapshot.
type Library struct {
	manifest schema.Manifest
	digest   string
	locale   string
	entries  map[recordKey]content.Entry
	byKind   map[content.Kind][]recordKey
}

// Load strictly decodes and validates one manifest and every JSON file below
// the entries directory. The returned library is usable only when the whole
// snapshot satisfies the Codex contract.
func Load(options Options) (*Library, error) {
	if options.FS == nil {
		return nil, errors.New("contentjson: FS is required")
	}
	locale := strings.TrimSpace(options.Locale)
	if locale == "" {
		locale = content.DefaultLocale
	}
	if !content.ValidLocale(locale) {
		return nil, fmt.Errorf("contentjson: locale %q is not a BCP 47 tag", locale)
	}
	manifestPath := cleanPath(options.ManifestPath, defaultManifestPath)
	entriesDir := cleanPath(options.EntriesDir, defaultEntriesDir)

	manifestData, err := fs.ReadFile(options.FS, manifestPath)
	if err != nil {
		return nil, fmt.Errorf("contentjson: read manifest %q: %w", manifestPath, err)
	}
	manifest, err := DecodeManifest(bytes.NewReader(manifestData))
	if err != nil {
		return nil, fmt.Errorf("contentjson: manifest %q: %w", manifestPath, err)
	}
	digest, err := manifest.DigestV2()
	if err != nil {
		return nil, fmt.Errorf("contentjson: manifest digest: %w", err)
	}

	entries := make(map[recordKey]content.Entry)
	byKind := make(map[content.Kind][]recordKey)
	collection := validate.Collection{}
	err = fs.WalkDir(options.FS, entriesDir, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path.Ext(name) != ".json" {
			return nil
		}
		data, readErr := fs.ReadFile(options.FS, name)
		if readErr != nil {
			return fmt.Errorf("read: %w", readErr)
		}
		values, decodeErr := decodeEntries(data, locale)
		if decodeErr != nil {
			return fmt.Errorf("%s: %w", name, decodeErr)
		}
		for _, value := range values {
			key := recordKey{id: value.ID, locale: value.Locale}
			if _, exists := entries[key]; exists {
				return fmt.Errorf("%s: duplicate entry key (%q, %q)", name, value.ID, value.Locale)
			}
			resource, exists := manifest.Resource(value.Kind)
			if !exists {
				return fmt.Errorf("%s: entry kind %q is not declared by the manifest", name, value.Kind)
			}
			if validateErr := resource.ValidateEntry(value); validateErr != nil {
				return fmt.Errorf("%s: %w", name, validateErr)
			}
			entries[key] = value
			byKind[value.Kind] = append(byKind[value.Kind], key)
			collection.Entries = append(collection.Entries, validate.RecordFile{
				File: name,
				Data: value,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("contentjson: load entries %q: %w", entriesDir, err)
	}
	for kind := range byKind {
		slices.SortFunc(byKind[kind], compareKeys)
	}
	if err := validateReferences(manifest, entries); err != nil {
		return nil, fmt.Errorf("contentjson: references: %w", err)
	}
	if issues := validation.ErrorsOnly(validate.CollectionV04(collection, validate.Options{Manifest: &manifest})); len(issues) > 0 {
		return nil, fmt.Errorf("contentjson: %w", issues[0].AsError())
	}

	return &Library{
		manifest: manifest,
		digest:   digest,
		locale:   locale,
		entries:  entries,
		byKind:   byKind,
	}, nil
}

// DecodeManifest strictly decodes and validates one Codex manifest.
func DecodeManifest(reader io.Reader) (schema.Manifest, error) {
	var manifest schema.Manifest
	if err := decodeStrict(reader, &manifest); err != nil {
		return schema.Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return schema.Manifest{}, err
	}
	return manifest, nil
}

// DecodeEntry strictly decodes one Codex entry. v0.4 JSON is validated as-is.
// v0.3 JSON is converted with migrate.EntryJSON for content.DefaultLocale, or
// for the only locale present in the document.
func DecodeEntry(reader io.Reader) (content.Entry, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return content.Entry{}, err
	}
	entries, err := decodeEntries(data, content.DefaultLocale)
	if err != nil {
		return content.Entry{}, err
	}
	if len(entries) == 0 {
		return content.Entry{}, errors.New("entry JSON is empty")
	}
	picked, _, ok := content.Pick(entries, entries[0].ID, content.DefaultLocale, "")
	if !ok {
		return entries[0], nil
	}
	return picked, nil
}

// Manifest returns a detached copy of the loaded manifest.
func (library *Library) Manifest() schema.Manifest {
	return library.manifest.Clone()
}

// ManifestDigest returns the Codex v0.4 canonical digest of the loaded
// manifest (codex-manifest/v2).
func (library *Library) ManifestDigest() string {
	return library.digest
}

// Len returns the number of (id, locale) records in the snapshot.
func (library *Library) Len() int {
	return len(library.entries)
}

// Get returns a detached entry by identifier, using content.Pick with the
// library default locale.
func (library *Library) Get(id content.ID) (content.Entry, bool) {
	picked, _, ok := content.Pick(library.all(), id, library.locale, "")
	return picked, ok
}

// GetLocale returns a detached entry for the exact (id, locale) record key.
func (library *Library) GetLocale(id content.ID, locale string) (content.Entry, bool) {
	entry, exists := library.entries[recordKey{id: id, locale: locale}]
	if !exists {
		return content.Entry{}, false
	}
	return entry.Clone(), true
}

// Locales returns the sorted locale tags stored for id.
func (library *Library) Locales(id content.ID) []string {
	return content.Locales(library.all(), id)
}

// Entries returns every detached record, ordered by (id, locale).
func (library *Library) Entries() []content.Entry {
	keys := make([]recordKey, 0, len(library.entries))
	for key := range library.entries {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, compareKeys)
	return library.cloneEntries(keys)
}

// ByKind returns detached records of kind, ordered by (id, locale).
func (library *Library) ByKind(kind content.Kind) []content.Entry {
	return library.cloneEntries(library.byKind[kind])
}

func (library *Library) cloneEntries(keys []recordKey) []content.Entry {
	entries := make([]content.Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, library.entries[key].Clone())
	}
	return entries
}

func (library *Library) all() []content.Entry {
	entries := make([]content.Entry, 0, len(library.entries))
	for _, entry := range library.entries {
		entries = append(entries, entry)
	}
	return entries
}

func compareKeys(left, right recordKey) int {
	if left.id != right.id {
		return strings.Compare(string(left.id), string(right.id))
	}
	return strings.Compare(left.locale, right.locale)
}

func cleanPath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return path.Clean(value)
}

func decodeEntries(data []byte, fallbackLocale string) ([]content.Entry, error) {
	raw, err := decodeDynamic(data)
	if err != nil {
		return nil, err
	}
	if looksLikeV03(raw) {
		return decodeV03Entries(data, raw, fallbackLocale)
	}
	entry, err := decodeV04Entry(data)
	if err != nil {
		return nil, err
	}
	return []content.Entry{entry}, nil
}

func decodeV04Entry(data []byte) (content.Entry, error) {
	var entry content.Entry
	if err := decodeStrict(bytes.NewReader(data), &entry); err != nil {
		return content.Entry{}, err
	}
	entry = normalizeEntry(entry)
	if err := entry.Validate(); err != nil {
		return content.Entry{}, err
	}
	return entry, nil
}

func decodeV03Entries(data []byte, raw any, fallbackLocale string) ([]content.Entry, error) {
	if err := rejectUnknownV03(raw); err != nil {
		return nil, err
	}
	locales := v03Locales(raw, fallbackLocale)
	entries := make([]content.Entry, 0, len(locales))
	seen := map[recordKey]struct{}{}
	for _, locale := range locales {
		entry, err := migrate.EntryJSON(data, locale)
		if err != nil {
			return nil, err
		}
		entry = normalizeEntry(entry)
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		key := recordKey{id: entry.ID, locale: entry.Locale}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, entry)
	}
	return entries, nil
}

func looksLikeV03(raw any) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if _, has := obj["locales"]; has {
		return true
	}
	if _, isObject := obj["title"].(map[string]any); isObject {
		return true
	}
	if _, isObject := obj["slug"].(map[string]any); isObject {
		return true
	}
	_, hasLocale := obj["locale"]
	return !hasLocale
}

var v03EntryFields = map[string]struct{}{
	"id": {}, "kind": {}, "status": {}, "visibility": {},
	"slug": {}, "title": {}, "content": {}, "excerpt": {},
	"author_id": {}, "parent_id": {}, "featured_media_id": {},
	"template": {}, "source": {}, "metadata": {}, "terms": {},
	"version": {}, "created_at": {}, "updated_at": {},
	"published_at": {}, "deleted_at": {}, "locales": {},
}

func rejectUnknownV03(raw any) error {
	obj, ok := raw.(map[string]any)
	if !ok {
		return errors.New("entry must be a JSON object")
	}
	for key := range obj {
		if _, known := v03EntryFields[key]; !known {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	return nil
}

func v03Locales(raw any, fallback string) []string {
	obj, _ := raw.(map[string]any)
	set := map[string]struct{}{}
	add := func(value string) {
		if content.ValidLocale(value) {
			set[value] = struct{}{}
		}
	}
	if locales, ok := obj["locales"].(map[string]any); ok {
		for locale := range locales {
			add(locale)
		}
	}
	for _, field := range []string{"slug", "title", "content", "excerpt"} {
		text, ok := obj[field].(map[string]any)
		if !ok {
			continue
		}
		for locale := range text {
			add(locale)
		}
	}
	out := make([]string, 0, len(set))
	for locale := range set {
		out = append(out, locale)
	}
	slices.Sort(out)
	if len(out) == 0 {
		if fallback == "" {
			fallback = content.DefaultLocale
		}
		return []string{fallback}
	}
	return out
}

func normalizeEntry(entry content.Entry) content.Entry {
	entry.ID = rewriteNumericID(string(entry.ID), entry.Kind)
	if entry.ParentID != "" {
		entry.ParentID = rewriteNumericID(string(entry.ParentID), entry.Kind)
	}
	if entry.FeaturedMediaID != "" {
		entry.FeaturedMediaID = string(rewriteNumericID(entry.FeaturedMediaID, "media"))
	}
	if entry.Kind == "media" {
		entry.Metadata = rewriteMediaAttachedTo(entry.Metadata)
	}
	return entry
}

func rewriteNumericID(id string, kind content.Kind) content.ID {
	if id == "" || !isAllDigits(id) {
		return content.ID(id)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return content.ID(id)
	}
	return content.ID(content.IDFromKindInt(kind, n))
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func rewriteMediaAttachedTo(metadata map[string]content.MetadataValue) map[string]content.MetadataValue {
	if metadata == nil {
		return nil
	}
	if _, exists := metadata["attached_to"]; exists {
		return metadata
	}
	post, exists := metadata["post"]
	if !exists {
		return metadata
	}
	attached, ok := attachedToValue(post.Value)
	if !ok {
		return metadata
	}
	cloned := make(map[string]content.MetadataValue, len(metadata))
	for key, value := range metadata {
		if key == "post" {
			continue
		}
		cloned[key] = value
	}
	cloned["attached_to"] = content.MetadataValue{Value: attached, Private: post.Private}
	return cloned
}

func attachedToValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || text == "0" {
			return "", false
		}
		if isAllDigits(text) {
			n, err := strconv.ParseInt(text, 10, 64)
			if err != nil || n <= 0 {
				return "", false
			}
			return content.IDFromKindInt(content.KindPost, n), true
		}
		return text, true
	case json.Number:
		n, err := typed.Int64()
		if err != nil || n <= 0 {
			return "", false
		}
		return content.IDFromKindInt(content.KindPost, n), true
	case float64:
		n := int64(typed)
		if typed != float64(n) || n <= 0 {
			return "", false
		}
		return content.IDFromKindInt(content.KindPost, n), true
	case int64:
		if typed <= 0 {
			return "", false
		}
		return content.IDFromKindInt(content.KindPost, typed), true
	default:
		return "", false
	}
}

func decodeStrict(reader io.Reader, target any) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}

	// Some Codex values have custom JSON decoders. A semantic round trip also
	// catches unknown fields nested inside those values.
	encoded, err := json.Marshal(target)
	if err != nil {
		return err
	}
	original, err := decodeDynamic(data)
	if err != nil {
		return err
	}
	roundTrip, err := decodeDynamic(encoded)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(original, roundTrip) {
		return errors.New("JSON contains unknown, redundant, or unstable fields")
	}
	return nil
}

func decodeDynamic(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func validateReferences(manifest schema.Manifest, entries map[recordKey]content.Entry) error {
	byID := make(map[content.ID][]content.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = append(byID[entry.ID], entry)
	}
	for _, entry := range entries {
		resource, _ := manifest.Resource(entry.Kind)
		taxonomies := make(map[string]struct{}, len(resource.Taxonomies))
		for _, taxonomy := range resource.Taxonomies {
			taxonomies[taxonomy] = struct{}{}
		}
		for _, term := range entry.Terms {
			if _, exists := taxonomies[term.Taxonomy]; !exists {
				return fmt.Errorf(
					"entry %q uses taxonomy %q, which is not assigned to kind %q",
					entry.ID,
					term.Taxonomy,
					entry.Kind,
				)
			}
		}
		for _, relation := range resource.Record.Relations {
			stored, exists := entry.Metadata[string(relation.ID)]
			if !exists || stored.Value == nil {
				continue
			}
			ids := relationIDs(stored.Value)
			for _, id := range ids {
				targets := byID[id]
				if len(targets) == 0 {
					return fmt.Errorf("entry %q relation %q references missing entry %q", entry.ID, relation.ID, id)
				}
				if targets[0].Kind != content.Kind(relation.Target) {
					return fmt.Errorf(
						"entry %q relation %q targets kind %q, entry %q has kind %q",
						entry.ID,
						relation.ID,
						relation.Target,
						id,
						targets[0].Kind,
					)
				}
			}
		}
		if entry.ParentID != "" {
			parents := byID[entry.ParentID]
			if len(parents) == 0 {
				return fmt.Errorf("entry %q references missing parent %q", entry.ID, entry.ParentID)
			}
			if parents[0].Kind != entry.Kind {
				return fmt.Errorf("entry %q parent %q has kind %q, expected %q", entry.ID, entry.ParentID, parents[0].Kind, entry.Kind)
			}
		}
	}
	return validateParentCycles(entries)
}

func validateParentCycles(entries map[recordKey]content.Entry) error {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[content.ID]int)
	var visit func(content.ID) error
	visit = func(id content.ID) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("parent cycle includes entry %q", id)
		case visited:
			return nil
		}
		state[id] = visiting
		for key, entry := range entries {
			if key.id != id {
				continue
			}
			if parent := entry.ParentID; parent != "" {
				if err := visit(parent); err != nil {
					return err
				}
			}
			break
		}
		state[id] = visited
		return nil
	}
	seen := map[content.ID]struct{}{}
	for key := range entries {
		if _, exists := seen[key.id]; exists {
			continue
		}
		seen[key.id] = struct{}{}
		if err := visit(key.id); err != nil {
			return err
		}
	}
	return nil
}

func relationIDs(value any) []content.ID {
	if id, ok := value.(string); ok {
		return []content.ID{content.ID(id)}
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array) {
		return nil
	}
	ids := make([]content.ID, 0, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		item := reflected.Index(index).Interface()
		switch id := item.(type) {
		case string:
			ids = append(ids, content.ID(id))
		case content.ID:
			ids = append(ids, id)
		}
	}
	return ids
}

// CanonicalDigest returns the codex-entry/v1 digest of one Entry JSON document.
// It uses canonical.Canonicalize; encoding/json is never used for the bytes.
func CanonicalDigest(entryJSON string) (string, error) {
	if _, err := canonical.Canonicalize(entryJSON); err != nil {
		return "", err
	}
	return canonical.Digest(entryJSON)
}
