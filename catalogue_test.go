package catalogue

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedRegistryMetadataAndExactSymbols(t *testing.T) {
	t.Parallel()
	registry := Default()
	if registry.Schema() != 1 || registry.Revision() != 1 {
		t.Fatalf("version = schema %d revision %d", registry.Schema(), registry.Revision())
	}
	source := registry.Source()
	if source.URL != "https://en.wikipedia.org/wiki/Catalogues_of_classical_compositions" {
		t.Fatalf("source URL = %q", source.URL)
	}
	if _, err := time.Parse(time.RFC3339, source.RetrievedAt); err != nil {
		t.Fatalf("retrieved_at = %q: %v", source.RetrievedAt, err)
	}

	symbols := registry.Symbols()
	if len(symbols) != 130 {
		t.Fatalf("symbol count = %d, want 130", len(symbols))
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(symbols, "\n")+"\n")))
	const wantHash = "d7653676d031cea71752f35e35c2b50153f55e73b8a493d597805a2a157c8883"
	if hash != wantHash {
		t.Fatalf("symbol registry hash = %s, want %s", hash, wantHash)
	}
	if !sort.StringsAreSorted(symbols) {
		t.Fatal("embedded symbols are not sorted")
	}
	if !contains(symbols, "ČW") || !contains(symbols, "Hob.") || !contains(symbols, "Krebs-WV") {
		t.Fatalf("Unicode or punctuated symbols missing from %#v", symbols)
	}

	symbols[0] = "mutated"
	if Default().Symbols()[0] != "A" {
		t.Fatal("Symbols exposed mutable registry storage")
	}
}

func TestEveryRegisteredSymbolParses(t *testing.T) {
	t.Parallel()
	registry := Default()
	for _, symbol := range registry.Symbols() {
		symbol := symbol
		t.Run(symbol, func(t *testing.T) {
			t.Parallel()
			references := registry.Parse(symbol + " 1")
			if len(references) != 1 || references[0].Identifier != "1" {
				t.Fatalf("Parse(%q) = %#v", symbol+" 1", references)
			}
		})
	}
}

func TestParseCatalogueReferenceGrammar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		text       string
		symbol     string
		marker     string
		identifier string
	}{
		{name: "basic", text: "Suite BWV 1007 performed live", symbol: "BWV", identifier: "1007"},
		{name: "marker and separator", text: "BWV Anh. 159", symbol: "BWV", marker: "anh", identifier: "anh159"},
		{name: "compact dotted marker", text: "BWV Anh.159", symbol: "BWV", marker: "anh", identifier: "anh159"},
		{name: "compact marker", text: "BWV Anh159", symbol: "BWV", marker: "anh", identifier: "anh159"},
		{name: "letter led core", text: "Hob. XVI:52", symbol: "Hob.", identifier: "xvi:52"},
		{name: "mixed core", text: "TWV 51:G9", symbol: "TWV", identifier: "51:g9"},
		{name: "letter suffix", text: "K 626a", symbol: "K", identifier: "626a"},
		{name: "slash", text: "Wq 182/3", symbol: "Wq", identifier: "182/3"},
		{name: "Unicode symbol fold", text: "čw 12", symbol: "ČW", identifier: "12"},
		{name: "equivalent dotted S", text: "s. 463", symbol: "S", identifier: "463"},
		{name: "hyphenated symbol", text: "KREBS-wv 4", symbol: "Krebs-WV", identifier: "4"},
		{name: "punctuation boundary", text: "BWV 1007, Cello Suite", symbol: "BWV", identifier: "1007"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			references := Parse(tt.text)
			if len(references) != 1 {
				t.Fatalf("Parse(%q) = %#v", tt.text, references)
			}
			wantSymbol := foldString(tt.symbol)
			want := Reference{Symbol: wantSymbol, Marker: tt.marker, Identifier: tt.identifier}
			if references[0] != want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.text, references[0], want)
			}
		})
	}
}

func TestParseRejectsInvalidReferences(t *testing.T) {
	t.Parallel()
	tests := []string{
		"BWV １００７",
		"BWV 100é",
		"BWV Alpha",
		"BWV 10_07",
		"BWV 10+07",
		"BWV 12::3",
		"BWV Anh.  159",
		"BWV 12345678901234567",
		"xBWV 1007",
		"BWVx 1007",
		"BWV-1007",
	}
	for _, text := range tests {
		text := text
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			if got := Parse(text); len(got) != 0 {
				t.Fatalf("Parse(%q) = %#v, want no references", text, got)
			}
		})
	}
}

func TestSharedReferenceRequiresCompleteEquality(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		left, right string
		want        bool
	}{
		{name: "same", left: "Cello Suite BWV 1007", right: "Bach: bwv 1007 live", want: true},
		{name: "marker normalized", left: "BWV Anh. 159", right: "bwv anh159", want: true},
		{name: "periods normalized", left: "Hob. XVI.52", right: "hob. xvi52", want: true},
		{name: "dotted S normalized", left: "S. 463", right: "s 463", want: true},
		{name: "multiple reference intersection", left: "BWV 1007 / BWV 1008", right: "BWV 1009 and BWV 1008", want: true},
		{name: "different identifier", left: "BWV 1007", right: "BWV 1008"},
		{name: "different symbol", left: "K 626", right: "KV 626"},
		{name: "different marker", left: "BWV Anh.159", right: "BWV App.159"},
		{name: "partial identifier", left: "BWV 1007", right: "BWV 1007a"},
		{name: "no references", left: "Cello Suite", right: "Cello Suite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SharedReference(tt.left, tt.right); got != tt.want {
				t.Fatalf("SharedReference(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestLongestSymbolWins(t *testing.T) {
	t.Parallel()
	references := Parse("AWV 12 and A 12 and KK 3")
	if len(references) != 3 {
		t.Fatalf("references = %#v", references)
	}
	if references[0].Symbol != foldString("AWV") || references[1].Symbol != foldString("A") || references[2].Symbol != foldString("KK") {
		t.Fatalf("symbols = %#v", references)
	}
}

func TestDecodeStrictValidation(t *testing.T) {
	t.Parallel()
	valid := func(symbols string) string {
		return `{"schema":1,"revision":1,"source":{"url":"https://example.test/catalogues","retrieved_at":"2026-07-15T00:00:00Z"},"symbols":` + symbols + `}`
	}
	registry, err := Decode([]byte(valid(`["A","ČW"]`)))
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Symbols(); !reflect.DeepEqual(got, []string{"A", "ČW"}) {
		t.Fatalf("symbols = %#v", got)
	}

	invalid := map[string]string{
		"malformed JSON":       `{`,
		"unknown root field":   strings.TrimSuffix(valid(`["A"]`), `}`) + `,"extra":true}`,
		"unknown source field": `{"schema":1,"revision":1,"source":{"url":"https://example.test","retrieved_at":"2026-07-15T00:00:00Z","extra":true},"symbols":["A"]}`,
		"trailing JSON":        valid(`["A"]`) + `{}`,
		"wrong schema":         strings.Replace(valid(`["A"]`), `"schema":1`, `"schema":2`, 1),
		"zero revision":        strings.Replace(valid(`["A"]`), `"revision":1`, `"revision":0`, 1),
		"invalid URL":          strings.Replace(valid(`["A"]`), `https://example.test/catalogues`, `relative`, 1),
		"invalid timestamp":    strings.Replace(valid(`["A"]`), `2026-07-15T00:00:00Z`, `yesterday`, 1),
		"empty symbols":        valid(`[]`),
		"unsorted":             valid(`["B","A"]`),
		"duplicate":            valid(`["A","A"]`),
		"case fold duplicate":  valid(`["KK","Kk"]`),
		"dotted duplicate":     valid(`["S","S."]`),
		"empty symbol":         valid(`[""]`),
		"numeric symbol":       valid(`["A1"]`),
		"unsupported symbol":   valid(`["A_B"]`),
		"leading punctuation":  valid(`[".A"]`),
		"trailing hyphen":      valid(`["A-"]`),
		"internal period":      valid(`["A.B"]`),
	}
	for name, data := range invalid {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode([]byte(data)); err == nil {
				t.Fatalf("Decode accepted %s", data)
			}
		})
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
