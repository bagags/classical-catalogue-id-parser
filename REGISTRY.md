# Registry and parser behaviour

The package recognizes work references such as `BWV 1007`, `Hob. XVI:52`,
and `Wq 182/3`. It uses only the Go standard library and reads no runtime
configuration or network data.

## Registry provenance and versions

`registry.v1.json` is embedded in the package. Its original 130 symbols were
audited from the complete Symbol column of Wikipedia's
[Catalogues of classical compositions](https://en.wikipedia.org/wiki/Catalogues_of_classical_compositions)
on 2026-07-15 at 16:20:03 UTC. Blank cells and aliases found only in the Notes
column were ignored. Compound Symbol cells were split, the `Opp.` and `WoO`
symbols were extracted from their numbered ranges, and equivalent `S`/`S.` and
case-insensitive `KK`/`Kk` spellings were each stored once. The resulting
revision contained 130 sorted symbols, including Unicode `ČW` and punctuated
forms such as `Hob.`, `Opp.`, and `Krebs-WV`.

Revision 2 was audited on 2026-08-02 against the supplied
`catalogue-series.jsonl` MusicBrainz snapshot (SHA-256
`97f084f10722edcbbeb256c876bd74794d392847d257b87110ec9ccfe7455637`).
The snapshot contains 627 catalogue series, 614 with numbered works, and
38,077 non-empty work-number relations. It added 41 catalogue prefixes that
were absent from the original source and moved `KV` from the canonical list to
an alias of `K`, producing 170 canonical symbols and 9 explicit aliases.
The added canonical symbols are `BC`, `BNB`, `BVN`, `Bryan`, `CD`, `CFF`,
`DürG`, `E`, `FXWM`, `FbWV`, `Fr`, `GB`, `GMW`, `H-U`, `HC`, `HUL`,
`HartW`, `JA`, `JB`, `JS`, `JWM`, `LV`, `MV`, `NWV`, `OM`, `PC`, `PadK`,
`RCT`, `RMWV`, `RT`, `ScharWV`, `Sk`, `Skb`, `SwWV`, `VWV`, `WFV`, `WKO`,
`ZD`, `ZN`, `ZS`, and `ZT`.

The audit selected a new canonical symbol only when a work-number prefix was
corroborated by its catalogue series name, disambiguation, alias, or a repeated
consistent prefix. It excluded bare numbers, standalone `Anh.` (an appendix
marker without its parent catalogue), generic `Nr.`, prose placeholders, and
isolated malformed values. This is deliberately conservative: a recognized
symbol says that the spelling is catalogue-like, not that its identifier is
globally unique.

Measured with `Parse` over the snapshot's 38,077 work-number strings, revision
1 found at least one reference in 12,877 of them (33.82%) and revision 2 finds
at least one reference in 34,098 (89.55%). These are per-input counts, not
emitted-reference counts: a string that contains several references counts
once here but yields several parsed references, so the number of emitted
`Reference` values is higher. Of the 34,239 strings beginning with a
letter-based prefix, revision 2 finds at least one reference in 34,093
(99.57%). It finds at least one reference in 568 of the 614 series that have
numbered works (92.51%). These are extraction-coverage measurements, not
precision estimates or claims that references reused by different composers
are equivalent.

Revision 3 audited colon usage in the supplied `catalogue-references.jsonl`
and `catalogue-works.jsonl` snapshots (SHA-256
`9d9bf1b2200d08069901379140729a840bb334201ce9f4943128d4861f7188df` and
`9e0b6b371d6ce6c4b753d5117d90681fd7ba405c78c13bfc92f2575ea58c699c`).
Of 38,077 authoritative work-number relations, 2,273 contain a colon. They
belong to 15 recognized canonical symbols: `CSWV`, `DLR`, `ED`, `FWV`,
`FXWM`, `GraunWV`, `Hob.`, `JB`, `JWM`, `LMV`, `Opp.`, `PadK`, `QV`, `SV`,
and `TWV`. Every observed number has one structural colon except the nine
`GraunWV` values, which have two. Corresponding titles demonstrate why the
separator cannot be classified from whitespace or the following word alone:
the authoritative `TWV 40:202` appears as `TWV 40: 202`, `TWV 33:4` appears
as `TWV 33: No. 4`, while the authoritative `K. 543` is followed by
`: IV. Finale` in a recording title.

## Canonical aliases and spelling variants

The explicit mappings are:

| Accepted name | Canonical symbol | Basis |
| --- | --- | --- |
| `BR-CPEB` | `CPEB` | Bach-Repertorium catalogue title prefix |
| `BR-JCFB` | `JFCB` | observed title prefix and canonical registry spelling |
| `BR-JEB` | `JEB` | Bach-Repertorium catalogue title prefix |
| `BR-WFB` | `WFB` | Bach-Repertorium catalogue title prefix |
| `CW` | `ČW` | isolated ASCII spelling in the otherwise consistently `ČW` series |
| `D-WD` | `WD` | series metadata explicitly identifies `WD` as its short form |
| `KV` | `K` | MusicBrainz aliases both names to the Köchel catalogue |
| `Op` | `Opp.` | common singular opus spelling |
| `Opus` | `Opp.` | expanded opus spelling |

Aliases are matched case-insensitively. A final period is optional for every
canonical symbol and alias, so `D`/`D.`, `Sz`/`Sz.`, `Wq`/`Wq.`, and
`JML`/`JML.` normalize without separate alias entries. ASCII hyphens and the
common Unicode hyphen/dash characters found in metadata are equivalent in
symbols and markers. Thus `BR-CPEB` and `BR‑CPEB` take the same mapping.

Some short symbols are reused by unrelated catalogues: the snapshot contains,
for example, several distinct `B.`, `F.`, `H.`, `S.`, and `W.` series. The
package has no composer or catalogue-series context, so it preserves their
lexical canonical symbols. Callers must combine a parsed reference with artist
or other domain context before treating such values as globally identical.

The top-level `schema` changes only for an incompatible JSON structure or
parser contract change. `revision` increments whenever symbols, aliases,
grammar metadata, or provenance change without changing that structure. The
optional `aliases` and `colon_depths` arrays are additive within schema 1, so
schema-1 registries without them remain valid. Each sorted `colon_depths`
entry names a canonical symbol and the positive number of structural colons
observed in that catalogue's complete identifiers. Decoding rejects unknown
JSON fields, unsupported schemas, invalid provenance, empty or malformed
symbols, unsorted entries, aliases or colon depths for unknown canonical
symbols, non-positive depths, and duplicate matchers under the parser's
Unicode folding, period, and hyphen rules. The embedded registry is decoded
during package initialization so invalid shipped data fails immediately.

## Parser grammar and normalization

A reference consists of a registered symbol or alias and an ASCII identifier.
They are normally separated by one or more ASCII spaces; a digit-starting
identifier may also be compact, as in `W227` or `JML.001`. Symbol matching is
Unicode case-insensitive, accepts an optional final period and common hyphen
variants, checks Unicode letter/number and hyphen boundaries, and tries longer
registered names first. Every result contains the canonical symbol, never the
matched alias.

The identifier has these rules:

- Core segments contain one or more ASCII letters or digits and may be joined
  by `.`, `:`, `/`, `-`, or an internal comma. A comma may have one following
  ASCII space when the next segment starts with a digit, as in `xviii, 57`.
  The complete core must contain a digit.
- An optional marker contains one to three ASCII letters and an optional final
  period. It may directly precede a digit-starting core or use exactly one
  ASCII separator space. Thus `Anh.159` and `Anh. 159` are equivalent, while
  two separator spaces are invalid. Observed longer markers `Coll`, `deest`,
  `Suppl`, `A-Juv`, and `B-Inc` are also accepted.
- The sectioned markers `Anh`, `App`, and `Suppl` may introduce a letter-led
  section followed by a number, as in `Anh. II 74`, or the observed auxiliary
  section form `App C, S. 714`.
- Letter-led compact cores remain cores when the leading letters are followed
  by a connector, so `XVI:52` is not split into a marker and another core.
- A colon followed by one ASCII space is resolved using the embedded
  catalogue grammar. Before the catalogue's structural colon depth is met,
  the colon and space are normalized into the identifier, as in
  `TWV 40: 202`. A continuation that does not fit the core grammar is rejected
  instead of emitting a partial reference, and a spaced structural suffix
  must contain its own digit. Consequently neither `TWV 33: No. 4` nor
  `TWV 33: Fantasia No. 4` becomes the misleading `TWV 33`. Once the
  structural depth is met, or for catalogues with no observed structural
  colon, the spaced colon terminates the identifier. Thus
  `K. 543: IV. Finale` yields `K 543`, and `TWV 40:202: IV. Allegro` yields
  `TWV 40:202`. Compact colons retain the existing connector grammar.
- Whitespace after the numeric core terminates the identifier instead of
  consuming following title words.
- Unicode identifier letters/numbers, unsupported punctuation, empty segments,
  and normalized identifiers longer than 16 bytes are rejected. This keeps
  malformed and prose-like MusicBrainz values out; superscript and Greek
  identifier suffixes remain outside the accepted grammar.

Equality requires the symbol, marker, and complete identifier to match.
Symbols and aliases use Unicode folding; marker and identifier letters use
ASCII folding; identifier periods and permitted spaces are removed; hyphen
variants normalize to ASCII `-`; commas and other connectors remain
significant. If either title contains several references, any exact
intersection counts as shared.

## Origin and compatibility

Revision 1 was moved from `music2bb`'s `internal/catalogue` package, originally
introduced in music2bb commit `61b470184b510e25f8736f382e794a99d6bdd261`.
Revision 2 preserves the original `Reference`, `Parse`, `SharedReference`,
`Decode`, and registry metadata APIs. It adds `Alias` and `Registry.Aliases`.
Canonical alias resolution intentionally changes equality for equivalent
spellings, most notably `K` and `KV`. Revision 3 preserves all public APIs and
adds only the optional, validated `colon_depths` registry grammar.

The standalone module intentionally contains no `music2bb` matching profiles,
weights, score thresholds, or other application-specific concepts.
