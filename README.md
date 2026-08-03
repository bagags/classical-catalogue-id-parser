# Classical catalogue ID parser

`classical-catalogue-id-parser` is a dependency-free Go package for finding
and comparing classical-composition catalogue references such as `BWV 1007`,
`Hob. XVI:52`, and `Wq 182/3`.

The package embeds a versioned registry of 170 canonical catalogue symbols and
9 explicit aliases. It performs no network or filesystem I/O at runtime and
has no music-player, search, or scoring concepts in its API.

## Usage

```go
package main

import (
	"fmt"

	catalogue "github.com/bagags/classical-catalogue-id-parser"
)

func main() {
	references := catalogue.Parse("Bach: Cello Suite BWV 1007")
	fmt.Println(references)

	sameWork := catalogue.SharedReference(
		"Cello Suite BWV 1007",
		"Bach: bwv 1007 live",
	)
	fmt.Println(sameWork)
}
```

`Parse` and `SharedReference` use the embedded default registry. `Default`
provides its immutable `Registry` value and metadata, while `Decode` strictly
validates a caller-supplied registry using the same schema and parser.

Parsed `Reference` fields are normalized comparison identities: symbols use
Unicode case folding, marker and identifier letters use ASCII case folding,
aliases resolve to canonical symbols, and permitted marker punctuation is
removed. For example, `K. 626` and `KV 626` both resolve to canonical symbol
`K`. See [REGISTRY.md](REGISTRY.md) for the complete grammar, normalization
rules, audit findings, version policy, and source provenance.

## Local MusicBrainz precision evaluation

The repository includes a dependency-free local review command. Point it at an
external `filtered/catalogue-references.jsonl` snapshot; the command reads that
file but never modifies or copies it into the repository.

```sh
go run ./cmd/catalogue-eval sample -input /path/to/filtered/catalogue-references.jsonl
go run ./cmd/catalogue-eval review
go run ./cmd/catalogue-eval summary
```

`sample` creates `.catalogue-eval/` with a source hash, deterministic sample,
and empty append-only judgment log, and refuses to overwrite an existing
evaluation. It samples parser outputs from relation `number` values and unique
work titles as separate populations. Use `review -id ID_PREFIX` to correct a
decision by appending a replacement; the latest judgment for an item wins.

Judge an output `valid` when the normalized symbol, marker, and complete
identifier are a genuine catalogue reference in the displayed MusicBrainz
context; use `invalid` for an emitted false positive or incorrect component,
`uncertain` when the available context or expertise is insufficient, and
`skip` when the item cannot be judged. Invalid and uncertain decisions require
one of the reasons offered by the command, and an `other` reason requires a
note.

The summary reports number and title precision separately. Decided precision
uses only representative `valid` and `invalid` judgments, with a 95% Wilson
interval; outcome bounds additionally treat uncertain and unreviewed items as
invalid or valid. Skips are excluded. Diagnostic items deliberately emphasize
rare symbols and shapes, so their counts are exploratory rather than
prevalence estimates. This workflow measures precision per emitted
`Reference`; zero-output inputs and parser misses are not sampled, so it does
not measure recall.

## Origin and compatibility

Revision 1 was extracted from `music2bb`'s `internal/catalogue` package.
Revision 2 retains its public parsing API and strict registry decoder while
expanding observed catalogue coverage and adding the `Alias` type and
`Registry.Aliases` method. Canonical alias resolution intentionally changes
comparison results for equivalent spellings such as `K` and `KV`.

## License

MIT. See [LICENSE](LICENSE).
