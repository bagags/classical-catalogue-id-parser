# Registry and parser behaviour

The package recognizes work references such as `BWV 1007`, `Hob. XVI:52`,
and `Wq 182/3`. It uses only the Go standard library and reads no runtime
configuration or network data.

## Registry provenance and versions

`registry.v1.json` is embedded in the package. Revision 1
was audited from the complete Symbol column of Wikipedia's
[Catalogues of classical compositions](https://en.wikipedia.org/wiki/Catalogues_of_classical_compositions)
on 2026-07-15 at 16:20:03 UTC. Blank cells and aliases found only in the Notes
column were ignored. Compound Symbol cells were split, the `Opp.` and `WoO`
symbols were extracted from their numbered ranges, and equivalent `S`/`S.` and
case-insensitive `KK`/`Kk` spellings were each stored once. The resulting
registry contains 130 sorted canonical symbols, including Unicode `ČW` and
punctuated forms such as `Hob.`, `Opp.`, and `Krebs-WV`.

Both `S` and `S.` input spellings resolve to the canonical `S` symbol; likewise,
Unicode case-insensitive matching accepts both `KK` and `Kk` for canonical
`KK`.

The top-level `schema` changes only for an incompatible JSON structure or
parser contract change. `revision` increments whenever symbols or provenance
change without changing that structure. Decoding rejects unknown JSON fields,
unsupported schemas, invalid provenance, empty or malformed symbols, unsorted
entries, and duplicates under the parser's Unicode folding rules. The embedded
registry is decoded during package initialization so invalid shipped data fails
immediately.

## Parser grammar and normalization

A reference consists of a registered symbol, at least one ASCII separator
space, and an ASCII identifier. Symbol matching is Unicode case-insensitive,
checks Unicode letter/number boundaries, and tries longer registered symbols
first.

The identifier has these rules:

- Core segments contain one or more ASCII letters or digits and may be joined
  without spaces by `.`, `:`, `/`, or `-`. The complete core must contain a
  digit.
- An optional marker contains one to three ASCII letters and an optional final
  period. It may directly precede a digit-starting core or use exactly one
  ASCII separator space. Thus `Anh.159` and `Anh. 159` are equivalent, while
  two separator spaces are invalid.
- Letter-led compact cores remain cores when the leading letters are followed
  by a connector, so `XVI:52` is not split into a marker and another core.
- Whitespace after the numeric core terminates the identifier instead of
  consuming following title words.
- Unicode identifier characters, unsupported punctuation, empty segments, and
  normalized identifiers longer than 16 characters are rejected.

Equality requires the symbol, marker, and complete identifier to match.
Symbols use Unicode folding; marker and identifier letters use ASCII folding;
identifier periods and its permitted marker separator space are removed. Other
connectors remain significant. If either title contains several references,
any exact intersection counts as shared.

## Extraction compatibility

The parser implementation, registry, and tests were moved byte-for-byte from
`music2bb`'s `internal/catalogue` package, originally introduced in music2bb
commit `61b470184b510e25f8736f382e794a99d6bdd261`. Its package name and public
API are unchanged. Only the Go module import path, registry location, and
documentation location changed for extraction; parsing and comparison
behaviour did not.

The standalone module intentionally contains no `music2bb` matching profiles,
weights, score thresholds, or other application-specific concepts.
