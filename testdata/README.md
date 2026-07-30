# Test Data

Files the tests read. Nothing here is part of the package.

```
testdata/
├── stories/     Z-machine story files
├── frotz/       save files made by Frotz
├── gargoyle/    save files made by Bocfel, under Gargoyle
├── jzip/        save files made by jzip, including uncompressed ones
├── local/       stories a maintainer fetched; gitignored
└── README.md
```

Saves are kept in a directory per interpreter rather than pooled, because which
program wrote a file is the most useful thing to know about it. Two
implementations disagreeing is the finding; a directory of mixed saves would
hide which one to suspect.

## stories/

Story files are the input side of the tests. The package never writes one. They
are here because a `CMem` chunk stores only the *changes* a game made to the
story's original dynamic memory, so a test cannot rebuild that memory without
the story it came from.

Each name carries the release number and serial number from the story's own
header:

```
zork1-r119-880429.z3
      ^^^^ ^^^^^^
      |    serial number
      release number
```

Saves only work with the exact story they were made from. Putting those two
numbers in the name makes a mismatched pair obvious at a glance, instead of
turning into a puzzling checksum error later. `TestStoryFixtures` checks every
name against the header inside the file, so a misnamed story fails right away.

| File | Game | Version | Release | Serial | Source |
|---|---|---|---|---|---|
| `zork1-r119-880429.z3` | Zork I | 3 | 119 | 880429 | [historicalsource/zork1](https://github.com/historicalsource/zork1), `COMPILED/zork1.z3` |
| `zork2-r63-860811.z3` | Zork II | 3 | 63 | 860811 | [historicalsource/zork2](https://github.com/historicalsource/zork2), `COMPILED/zork2.z3` |
| `zork3-r25-860811.z3` | Zork III | 3 | 25 | 860811 | [historicalsource/zork3](https://github.com/historicalsource/zork3), `COMPILED/zork3.z3` |

Microsoft, Team Xbox, and Activision released the compiled Zork files under the
MIT License. Each story file has its own license file next to it, and that file
is the one that applies. Read it before you reuse a story file for anything.

| File | License |
|---|---|
| `zork1-r119-880429.z3` | MIT, Copyright (c) 2025 Microsoft — `stories/LICENSE.zork1.txt` |
| `zork2-r63-860811.z3` | MIT, Copyright (c) 2025 Microsoft — `stories/LICENSE.zork2.txt` |
| `zork3-r25-860811.z3` | MIT, Copyright (c) 2025 Microsoft — `stories/LICENSE.zork3.txt` |

## frotz/

Saves written by Frotz, an established interpreter. They are what proves this
package reads files it did not write. See `frotz/README.md` for how to make
them.

## gargoyle/

Saves written by Bocfel, a second and unrelated implementation, running under
Gargoyle. Bocfel writes a considerably fuller file than Frotz — annotations,
its own unregistered chunk, a reference to the story file, and trailing bytes
past the FORM — so these fixtures cover several things Frotz cannot produce.
See `gargoyle/README.md`, including the note about the filesystem path embedded
in the `IntD` chunk.

## local/

Empty in a fresh clone, and ignored by git apart from its README and fetch
script. Z-machine versions other than 3 can be *fetched* even though they cannot
be shipped, and several deltas can only be settled by a story of another
version. `local/fetch.sh` downloads them; the tests use whatever is there and
skip when there is nothing. See `local/README.md`.

## jzip/

Saves written by jzip, a third interpreter, and the only one that can be told
to store dynamic memory uncompressed. Its files are also the plainest here —
three chunks, no annotations, nothing after the FORM. The same source tree
builds `ckifzs`, the conformance checker from standard 9.2, which is the only
tool in this repository that judges our output without being our output. See
`jzip/README.md`.

There is no `handbuilt/` directory any more. It held one uncompressed save,
made by this package because no interpreter available could produce one; jzip
now can, and a fixture an interpreter wrote is worth more than one we wrote.
Deliberately malformed files are still built inside `golden_test.go` rather than
committed, since §21 prefers a test that states its intent to an opaque blob.
