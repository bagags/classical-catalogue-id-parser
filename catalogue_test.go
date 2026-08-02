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
	if registry.Schema() != 1 || registry.Revision() != 2 {
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
	if len(symbols) != 170 {
		t.Fatalf("symbol count = %d, want 170", len(symbols))
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(symbols, "\n")+"\n")))
	const wantHash = "1bf917610e0483a6a01b8854b11fd434d58d89c053af04c1fd7c36b273bd3248"
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

	aliases := registry.Aliases()
	if len(aliases) != 9 || aliases[0] != (Alias{Name: "BR-CPEB", Symbol: "CPEB"}) {
		t.Fatalf("aliases = %#v", aliases)
	}
	aliases[0].Name = "mutated"
	if Default().Aliases()[0].Name != "BR-CPEB" {
		t.Fatal("Aliases exposed mutable registry storage")
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
		{name: "optional period", text: "Sz. 41", symbol: "Sz", identifier: "41"},
		{name: "period omitted", text: "Hob XVI:52", symbol: "Hob.", identifier: "xvi:52"},
		{name: "compact", text: "JML.001", symbol: "JML", identifier: "001"},
		{name: "hyphenated symbol", text: "KREBS-wv 4", symbol: "Krebs-WV", identifier: "4"},
		{name: "Unicode hyphen", text: "Krebs‑WV 4", symbol: "Krebs-WV", identifier: "4"},
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

func TestObservedAliasesResolveToCanonicalSymbols(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		text       string
		symbol     string
		marker     string
		identifier string
	}{
		{name: "Koechel dotted", text: "K. 626", symbol: "K", identifier: "626"},
		{name: "Koechel KV", text: "KV 626", symbol: "K", identifier: "626"},
		{name: "opus singular", text: "op. 38", symbol: "Opp.", identifier: "38"},
		{name: "opus word", text: "opus 38", symbol: "Opp.", identifier: "38"},
		{name: "Bach Repertorium CPEB", text: "BR‑CPEB C 54.1", symbol: "CPEB", marker: "c", identifier: "c541"},
		{name: "Bach Repertorium JCFB", text: "BR-JCFB A 45", symbol: "JFCB", marker: "a", identifier: "a45"},
		{name: "Bach Repertorium JEB", text: "BR‑JEB A 4", symbol: "JEB", marker: "a", identifier: "a4"},
		{name: "Bach Repertorium WFB", text: "BR‑WFB A 65", symbol: "WFB", marker: "a", identifier: "a65"},
		{name: "Schoenborn Wiesentheid", text: "D‑WD 573", symbol: "WD", identifier: "573"},
		{name: "ASCII Chaykovsky", text: "CW 424", symbol: "ČW", identifier: "424"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			references := Parse(tt.text)
			want := []Reference{{Symbol: foldString(tt.symbol), Marker: tt.marker, Identifier: tt.identifier}}
			if !reflect.DeepEqual(references, want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.text, references, want)
			}
		})
	}
}

func TestObservedCatalogueIdentifierForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text string
		want Reference
	}{
		{text: "W227", want: Reference{Symbol: "W", Identifier: "227"}},
		{text: "H. xviii, 57", want: Reference{Symbol: "H", Identifier: "xviii,57"}},
		{text: "FbWV Anh. IV/06", want: Reference{Symbol: foldString("FbWV"), Marker: "anh", Identifier: "anhiv/06"}},
		{text: "BWV App C, S. 714", want: Reference{Symbol: "BWV", Marker: "app", Identifier: "appc,s714"}},
		{text: "BWV Suppl 2, S. 642", want: Reference{Symbol: "BWV", Marker: "suppl", Identifier: "suppl2,s642"}},
		{text: "CNW Coll. 22", want: Reference{Symbol: "CNW", Marker: "coll", Identifier: "coll22"}},
		{text: "WAB deest 10", want: Reference{Symbol: "WAB", Marker: "deest", Identifier: "deest10"}},
		{text: "BR‑CPEB A‑Juv 6.3", want: Reference{Symbol: "CPEB", Marker: "a-juv", Identifier: "a-juv63"}},
		{text: "BNB I/B/9", want: Reference{Symbol: "BNB", Identifier: "i/b/9"}},
		{text: "PadK VII:8", want: Reference{Symbol: foldString("PadK"), Identifier: "vii:8"}},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()
			references := Parse(tt.text)
			if len(references) != 1 || references[0] != tt.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.text, references, tt.want)
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
		"BWV 12:,3",
		"BWV A-B 12",
		"BWV Anh.  159",
		"BWV 12345678901234567",
		"xBWV 1007",
		"BWVx 1007",
		"BWV-1007",
		"x-BWV 1007",
		"BR-X-CPEB Q 54.1",
		"BWV Alpha 123",
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
		{name: "canonical alias", left: "K 626", right: "KV 626", want: true},
		{name: "different symbol", left: "K 626", right: "KK 626"},
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
	withAliases := func(symbols, aliases string) string {
		return strings.TrimSuffix(valid(symbols), `}`) + `,"aliases":` + aliases + `}`
	}
	registry, err = Decode([]byte(withAliases(`["K"]`, `[{"name":"KV","symbol":"K"}]`)))
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.Parse("KV 626"); !reflect.DeepEqual(got, []Reference{{Symbol: "K", Identifier: "626"}}) {
		t.Fatalf("alias parse = %#v", got)
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
		"alias unknown field":  withAliases(`["K"]`, `[{"name":"KV","symbol":"K","extra":true}]`),
		"malformed alias":      withAliases(`["K"]`, `[{"name":"K_V","symbol":"K"}]`),
		"unknown alias target": withAliases(`["K"]`, `[{"name":"KV","symbol":"Q"}]`),
		"alias target is alias": withAliases(`["K"]`, `[`+
			`{"name":"L","symbol":"K"},{"name":"M","symbol":"L"}]`),
		"unsorted aliases": withAliases(`["K"]`, `[`+
			`{"name":"M","symbol":"K"},{"name":"L","symbol":"K"}]`),
		"equivalent alias": withAliases(`["K"]`, `[{"name":"K.","symbol":"K"}]`),
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
