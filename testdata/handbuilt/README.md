# Hand-built saves

Files this package produced itself, for the parts of §19's fixture list that no
interpreter available here can supply.

There is exactly one, and that is deliberate. A save written by Frotz or Bocfel
is evidence; a save written by us is only a restatement of what our own code
does, and a directory full of those would look like coverage while proving
nothing. The bar for adding one is that some real interpreter *cannot* produce
the file and the file tests something real.

## zork1-r119-umem.qzl

§19 asks for a valid uncompressed save. Frotz and Bocfel both compress by
default, and neither offers a way not to, so this is the only way to have one.

| | |
|---|---|
| Story | `../stories/zork1-r119-880429.z3` |
| State | identical to `../frotz/zork1-r119-kitchen.qzl` — the Kitchen, 4 moves |
| Built by | reading that save and writing it with `WithEncoding(MemoryUncompressed)` |
| Size | 11424 bytes, against 434 for the compressed original |

Its provenance is the point: it holds the *same game* as a real Frotz save, so
`TestGoldenUMem` can check that both files reconstruct to the same state. That
is the claim §18.1 makes about the encoding being a free choice, tested against
a file an interpreter actually wrote rather than against another of our own.

Frotz restores it. That was checked with:

```sh
dfrotz -p -m -w 80 -L zork1-r119-umem.qzl ../stories/zork1-r119-880429.z3
```

## What is not here, and why

Four deliberately non-conforming files were built while testing how strict this
package is compared to Frotz — chunks out of order, a missing dummy frame, an
over-long `IFhd`, and a duplicate `IFhd`. None is committed.

`TestGoldenNonConformingVariants` builds all four in memory instead, by reading
the committed Kitchen save and breaking it one way at a time. That is better
than committing them for two reasons: §21 asks tests to state the intended
result explicitly rather than lean on opaque binaries, and a file derived in the
test cannot drift out of step with the fixture it came from.

What those four established about Frotz's strictness — it accepts two of them and
refuses the other two, disagreeing with us in both directions — is recorded in
section 7 of `../../spec-deltas.md`, with the verdicts repeated in the test so
they stay attached to running code.
