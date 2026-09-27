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
	"strings"

	"github.com/fastygo/codex/content"
	"github.com/fastygo/codex/schema"
)

const (
	defaultManifestPath = "manifest.json"
	defaultEntriesDir   = "entries"
)

// Options selects fixture paths inside FS. Zero values use manifest.json and
// entries/*.json.
type Options struct {
	FS           fs.FS
	ManifestPath string
	EntriesDir   string
}

// Library is an immutable, in-memory Codex snapshot.
type Library struct {
	manifest schema.Manifest
	digest   string
	entries  map[content.ID]content.Entry
	byKind   map[content.Kind][]content.ID
}

// Load strictly decodes and validates one manifest and every JSON file below
// the entries directory. The returned library is usable only when the whole
// snapshot satisfies the Codex contract.
func Load(options Options) (*Library, error) {
	if options.FS == nil {
		return nil, errors.New("contentjson: FS is required")
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
	digest, err := manifest.Digest()
	if err != nil {
		return nil, fmt.Errorf("contentjson: manifest digest: %w", err)
	}

	entries := make(map[content.ID]content.Entry)
	byKind := make(map[content.Kind][]content.ID)
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
		value, decodeErr := DecodeEntry(bytes.NewReader(data))
		if decodeErr != nil {
			return fmt.Errorf("%s: %w", name, decodeErr)
		}
		if _, exists := entries[value.ID]; exists {
			return fmt.Errorf("%s: duplicate entry id %q", name, value.ID)
		}
		resource, exists := manifest.Resource(value.Kind)
		if !exists {
			return fmt.Errorf("%s: entry kind %q is not declared by the manifest", name, value.Kind)
		}
		if validateErr := resource.ValidateEntry(value); validateErr != nil {
			return fmt.Errorf("%s: %w", name, validateErr)
		}
		entries[value.ID] = value
		byKind[value.Kind] = append(byKind[value.Kind], value.ID)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("contentjson: load entries %q: %w", entriesDir, err)
	}
	for kind := range byKind {
		slices.Sort(byKind[kind])
	}
	if err := validateReferences(manifest, entries); err != nil {
		return nil, fmt.Errorf("contentjson: references: %w", err)
	}

	return &Library{
		manifest: manifest,
		digest:   digest,
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

// DecodeEntry strictly decodes and validates storage-independent Entry
// invariants. Load additionally validates it against the selected manifest.
func DecodeEntry(reader io.Reader) (content.Entry, error) {
	var entry content.Entry
	if err := decodeStrict(reader, &entry); err != nil {
		return content.Entry{}, err
	}
	if err := entry.Validate(); err != nil {
		return content.Entry{}, err
	}
	return entry, nil
}

// Manifest returns a detached copy of the loaded manifest.
func (library *Library) Manifest() schema.Manifest {
	return library.manifest.Clone()
}

// ManifestDigest returns the Codex canonical digest of the loaded manifest.
func (library *Library) ManifestDigest() string {
	return library.digest
}

// Len returns the number of entries in the snapshot.
func (library *Library) Len() int {
	return len(library.entries)
}

// Get returns a detached entry by identifier.
func (library *Library) Get(id content.ID) (content.Entry, bool) {
	entry, exists := library.entries[id]
	if !exists {
		return content.Entry{}, false
	}
	return entry.Clone(), true
}

// Entries returns every detached entry, ordered by identifier.
func (library *Library) Entries() []content.Entry {
	ids := make([]content.ID, 0, len(library.entries))
	for id := range library.entries {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return library.cloneEntries(ids)
}

// ByKind returns detached entries of kind, ordered by identifier.
func (library *Library) ByKind(kind content.Kind) []content.Entry {
	return library.cloneEntries(library.byKind[kind])
}

func (library *Library) cloneEntries(ids []content.ID) []content.Entry {
	entries := make([]content.Entry, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, library.entries[id].Clone())
	}
	return entries
}

func cleanPath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return path.Clean(value)
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

func validateReferences(manifest schema.Manifest, entries map[content.ID]content.Entry) error {
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
				target, exists := entries[id]
				if !exists {
					return fmt.Errorf("entry %q relation %q references missing entry %q", entry.ID, relation.ID, id)
				}
				if target.Kind != content.Kind(relation.Target) {
					return fmt.Errorf(
						"entry %q relation %q targets kind %q, entry %q has kind %q",
						entry.ID,
						relation.ID,
						relation.Target,
						id,
						target.Kind,
					)
				}
			}
		}
		if entry.ParentID != "" {
			parent, exists := entries[entry.ParentID]
			if !exists {
				return fmt.Errorf("entry %q references missing parent %q", entry.ID, entry.ParentID)
			}
			if parent.Kind != entry.Kind {
				return fmt.Errorf("entry %q parent %q has kind %q, expected %q", entry.ID, entry.ParentID, parent.Kind, entry.Kind)
			}
		}
	}
	return validateParentCycles(entries)
}

func validateParentCycles(entries map[content.ID]content.Entry) error {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[content.ID]int, len(entries))
	var visit func(content.ID) error
	visit = func(id content.ID) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("parent cycle includes entry %q", id)
		case visited:
			return nil
		}
		state[id] = visiting
		if parent := entries[id].ParentID; parent != "" {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[id] = visited
		return nil
	}
	for id := range entries {
		if err := visit(id); err != nil {
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
