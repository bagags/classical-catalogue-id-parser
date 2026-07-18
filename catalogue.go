// Package catalogue parses classical-composition catalogue references from a
// versioned, embedded symbol registry.
package catalogue

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

const currentSchema = 1

//go:embed registry.v1.json
var embeddedRegistry []byte

var defaultRegistry = mustDecode(embeddedRegistry)

// Source records where and when a registry revision was audited.
type Source struct {
	URL         string `json:"url"`
	RetrievedAt string `json:"retrieved_at"`
}

// Registry is a validated catalogue-symbol registry. Its contents are
// immutable after Decode returns.
type Registry struct {
	schema   int
	revision int
	source   Source
	symbols  []string
	matchers []symbolMatcher
}

// Reference is the normalized identity of one parsed catalogue reference.
// Identifier includes the marker, if present.
type Reference struct {
	Symbol     string
	Marker     string
	Identifier string
}

type registryJSON struct {
	Schema   int      `json:"schema"`
	Revision int      `json:"revision"`
	Source   Source   `json:"source"`
	Symbols  []string `json:"symbols"`
}

type symbolMatcher struct {
	name   string
	folded string
	runes  []rune
}

// Decode strictly decodes and validates a version-1 registry.
func Decode(data []byte) (*Registry, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw registryJSON
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode catalogue registry: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := validateRegistry(raw); err != nil {
		return nil, err
	}

	registry := &Registry{
		schema: raw.Schema, revision: raw.Revision, source: raw.Source,
		symbols:  append([]string(nil), raw.Symbols...),
		matchers: make([]symbolMatcher, 0, len(raw.Symbols)),
	}
	for _, symbol := range raw.Symbols {
		registry.matchers = append(registry.matchers, symbolMatcher{
			name: symbol, folded: foldString(symbol), runes: []rune(symbol),
		})
	}
	sort.Slice(registry.matchers, func(i, j int) bool {
		if len(registry.matchers[i].runes) == len(registry.matchers[j].runes) {
			return registry.matchers[i].name < registry.matchers[j].name
		}
		return len(registry.matchers[i].runes) > len(registry.matchers[j].runes)
	})
	return registry, nil
}

// Default returns the validated embedded registry. Registry methods do not
// expose mutable internal storage.
func Default() *Registry {
	return defaultRegistry
}

// Schema returns the registry's incompatible-format version.
func (r *Registry) Schema() int {
	return r.schema
}

// Revision returns the registry content and provenance revision.
func (r *Registry) Revision() int {
	return r.revision
}

// Source returns a copy of the registry provenance.
func (r *Registry) Source() Source {
	return r.source
}

// Symbols returns a caller-owned copy of the canonical symbol list.
func (r *Registry) Symbols() []string {
	return append([]string(nil), r.symbols...)
}

// Parse returns all valid catalogue references found in text.
func (r *Registry) Parse(text string) []Reference {
	runes := []rune(text)
	references := make([]Reference, 0)
	for start := 0; start < len(runes); start++ {
		if start > 0 && isBoundaryWordRune(runes[start-1]) {
			continue
		}
		for _, symbol := range r.matchers {
			symbolEnd := start + len(symbol.runes)
			if symbolEnd > len(runes) || !strings.EqualFold(string(runes[start:symbolEnd]), symbol.name) {
				continue
			}
			if symbol.name == "S" && symbolEnd < len(runes) && runes[symbolEnd] == '.' {
				symbolEnd++
			}
			if symbolEnd < len(runes) && isBoundaryWordRune(runes[symbolEnd]) {
				continue
			}
			identifierStart := symbolEnd
			for identifierStart < len(runes) && runes[identifierStart] == ' ' {
				identifierStart++
			}
			if identifierStart == symbolEnd {
				continue
			}
			marker, identifier, identifierEnd, ok := parseIdentifier(runes, identifierStart)
			if !ok {
				continue
			}
			references = append(references, Reference{
				Symbol: symbol.folded, Marker: marker, Identifier: identifier,
			})
			start = identifierEnd - 1
			break
		}
	}
	return references
}

// SharedReference reports whether both texts contain the same complete
// catalogue reference.
func (r *Registry) SharedReference(left, right string) bool {
	leftReferences := r.Parse(left)
	if len(leftReferences) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(leftReferences))
	for _, reference := range leftReferences {
		seen[reference.key()] = struct{}{}
	}
	for _, reference := range r.Parse(right) {
		if _, ok := seen[reference.key()]; ok {
			return true
		}
	}
	return false
}

// Parse uses the validated embedded registry.
func Parse(text string) []Reference {
	return defaultRegistry.Parse(text)
}

// SharedReference uses the validated embedded registry.
func SharedReference(left, right string) bool {
	return defaultRegistry.SharedReference(left, right)
}

func (r Reference) key() string {
	return r.Symbol + "\x00" + r.Marker + "\x00" + r.Identifier
}

func parseIdentifier(runes []rune, start int) (marker, identifier string, end int, ok bool) {
	markerEnd, coreStart, hasMarker := findMarker(runes, start)
	if !hasMarker {
		markerEnd = start
		coreStart = start
	}
	coreEnd, ok := parseCore(runes, coreStart)
	if !ok || !validIdentifierEnd(runes, coreEnd) {
		return "", "", start, false
	}

	var normalized strings.Builder
	for _, current := range runes[start:coreEnd] {
		switch {
		case current == '.' || current == ' ':
		case current >= 'A' && current <= 'Z':
			normalized.WriteRune(current + ('a' - 'A'))
		default:
			normalized.WriteRune(current)
		}
	}
	if normalized.Len() > 16 {
		return "", "", start, false
	}
	if hasMarker {
		marker = normalizeMarker(runes[start:markerEnd])
	}
	return marker, normalized.String(), coreEnd, true
}

func findMarker(runes []rune, start int) (markerEnd, coreStart int, ok bool) {
	end := start
	for end < len(runes) && end-start < 3 && isASCIILetter(runes[end]) {
		end++
	}
	if end == start {
		return 0, 0, false
	}
	markerEnd = end
	if markerEnd < len(runes) && runes[markerEnd] == '.' {
		markerEnd++
	}
	if markerEnd < len(runes) && isASCIIDigit(runes[markerEnd]) {
		return markerEnd, markerEnd, true
	}
	if markerEnd+1 < len(runes) && runes[markerEnd] == ' ' && isASCIIDigit(runes[markerEnd+1]) {
		return markerEnd, markerEnd + 1, true
	}
	return 0, 0, false
}

func parseCore(runes []rune, start int) (int, bool) {
	if start >= len(runes) || !isASCIIAlphanumeric(runes[start]) {
		return start, false
	}
	hasDigit := false
	end := start
	lastWasConnector := false
	for end < len(runes) {
		current := runes[end]
		switch {
		case isASCIIAlphanumeric(current):
			hasDigit = hasDigit || isASCIIDigit(current)
			lastWasConnector = false
			end++
		case isCoreConnector(current):
			if lastWasConnector || end+1 >= len(runes) || !isASCIIAlphanumeric(runes[end+1]) {
				return start, false
			}
			lastWasConnector = true
			end++
		default:
			if !hasDigit && unicode.IsSpace(current) {
				return start, false
			}
			return end, hasDigit && !lastWasConnector
		}
	}
	return end, hasDigit && !lastWasConnector
}

func validIdentifierEnd(runes []rune, end int) bool {
	if end >= len(runes) || unicode.IsSpace(runes[end]) {
		return true
	}
	if isBoundaryWordRune(runes[end]) || runes[end] == '_' {
		return false
	}
	return strings.ContainsRune(",;!?()[]{}\"'，。；！？（）【】《》–—", runes[end])
}

func validateRegistry(raw registryJSON) error {
	if raw.Schema != currentSchema {
		return fmt.Errorf("catalogue registry schema = %d, want %d", raw.Schema, currentSchema)
	}
	if raw.Revision < 1 {
		return fmt.Errorf("catalogue registry revision must be positive")
	}
	parsedURL, err := url.ParseRequestURI(raw.Source.URL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("catalogue registry source URL is invalid")
	}
	if _, err := time.Parse(time.RFC3339, raw.Source.RetrievedAt); err != nil {
		return fmt.Errorf("catalogue registry retrieved_at is invalid: %w", err)
	}
	if len(raw.Symbols) == 0 {
		return fmt.Errorf("catalogue registry symbols must not be empty")
	}
	folded := make(map[string]string, len(raw.Symbols))
	for index, symbol := range raw.Symbols {
		if err := validateSymbol(symbol); err != nil {
			return fmt.Errorf("catalogue registry symbol %q: %w", symbol, err)
		}
		if index > 0 && raw.Symbols[index-1] >= symbol {
			return fmt.Errorf("catalogue registry symbols must be sorted and unique")
		}
		key := foldString(strings.TrimSuffix(symbol, "."))
		if previous, exists := folded[key]; exists {
			return fmt.Errorf("catalogue registry symbols %q and %q are equivalent", previous, symbol)
		}
		folded[key] = symbol
	}
	return nil
}

func validateSymbol(symbol string) error {
	runes := []rune(symbol)
	if len(runes) == 0 {
		return fmt.Errorf("must not be empty")
	}
	if !unicode.IsLetter(runes[0]) {
		return fmt.Errorf("must start with a Unicode letter")
	}
	for index, current := range runes {
		switch {
		case unicode.IsLetter(current):
		case current == '.':
			if index != len(runes)-1 || index == 0 || !unicode.IsLetter(runes[index-1]) {
				return fmt.Errorf("period must follow the final letter")
			}
		case current == '-':
			if index == 0 || index == len(runes)-1 || !unicode.IsLetter(runes[index-1]) || !unicode.IsLetter(runes[index+1]) {
				return fmt.Errorf("hyphen must join Unicode letters")
			}
		default:
			return fmt.Errorf("contains unsupported character %q", current)
		}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode catalogue registry: multiple JSON values")
		}
		return fmt.Errorf("decode catalogue registry: %w", err)
	}
	return nil
}

func mustDecode(data []byte) *Registry {
	registry, err := Decode(data)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded catalogue registry: %v", err))
	}
	return registry
}

func normalizeMarker(value []rune) string {
	var normalized strings.Builder
	for _, current := range value {
		if current == '.' {
			continue
		}
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		normalized.WriteRune(current)
	}
	return normalized.String()
}

func foldString(value string) string {
	return strings.Map(foldRune, value)
}

func foldRune(value rune) rune {
	minimum := value
	for current := unicode.SimpleFold(value); current != value; current = unicode.SimpleFold(current) {
		if current < minimum {
			minimum = current
		}
	}
	return minimum
}

func isASCIILetter(value rune) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isASCIIDigit(value rune) bool {
	return value >= '0' && value <= '9'
}

func isASCIIAlphanumeric(value rune) bool {
	return isASCIILetter(value) || isASCIIDigit(value)
}

func isCoreConnector(value rune) bool {
	return value == '.' || value == ':' || value == '/' || value == '-'
}

func isBoundaryWordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsNumber(value)
}
