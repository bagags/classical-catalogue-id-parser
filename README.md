# Classical catalogue ID parser

`classical-catalogue-id-parser` is a dependency-free Go package for finding
and comparing classical-composition catalogue references such as `BWV 1007`,
`Hob. XVI:52`, and `Wq 182/3`.

The package embeds a versioned registry of 130 catalogue symbols. It performs
no network or filesystem I/O at runtime and has no music-player, search, or
scoring concepts in its API.

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
and permitted marker punctuation is removed. See [REGISTRY.md](REGISTRY.md)
for the complete grammar, normalization rules, version policy, and source
provenance.

## Extraction compatibility

The parser implementation, registry, and tests were extracted unchanged from
`music2bb`'s `internal/catalogue` package. The required extraction changes are
limited to the Go module import path, the registry's repository location, and
documentation ownership; parsing and comparison behaviour are unchanged.

## License

MIT. See [LICENSE](LICENSE).
