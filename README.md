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

## Origin and compatibility

Revision 1 was extracted from `music2bb`'s `internal/catalogue` package.
Revision 2 retains its public parsing API and strict registry decoder while
expanding observed catalogue coverage and adding the `Alias` type and
`Registry.Aliases` method. Canonical alias resolution intentionally changes
comparison results for equivalent spellings such as `K` and `KV`.

## License

MIT. See [LICENSE](LICENSE).
