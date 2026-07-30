# Local story files

Everything in this directory except this file is ignored by git, on purpose.

The committed fixtures are all Z-machine version 3, because Zork I, II, and III
are the only Infocom stories released under a licence that permits shipping them
(D43 in `../../spec-deltas.md`). Several deltas can only be settled by a story of
another version, and those stories exist — they are simply not ours to
redistribute. This directory is where a maintainer can put their own copy so the
tests have something to work with.

Nothing here is required. `go test ./...` passes with this directory empty;
`TestLocalStories` reports what it found and skips what it did not.

## What these files settle

| Story | Version | Settles |
|---|---|---|
| Border Zone (`spy.z5`) | 5 | **D16** — the `CALL_xN` discard bit, which cannot occur in a V3 save |
| Journey (`journey.z6`) | 6 | **D9** — a save with no dummy frame, which V6 does not write |
| `journey-r83-forest.glksave` | 6 | the save itself, made in Gargoyle; five frames, none of them a dummy |
| Beyond Zork (`bzbeta.z5`) | 5 | a second V5 opinion; 34800 bytes of dynamic memory, the largest to hand |

No version 1 or 2 story has been found in any form that could be fetched, so
**D27's trigger — a story with no checksum at `$1C` — remains untested by any
real file.** The `historicalsource` copies of the early games are all V3
re-releases.

## Fetching them

`historicalsource` on GitHub hosts the Infocom catalogue. **Only its `zork1`,
`zork2`, and `zork3` repositories carry a licence**; the rest have none, which
is exactly why these files are fetched rather than committed. Decide for
yourself whether to download them.

```sh
./fetch.sh            # from testdata/local/
```

Or by hand, with the `gh` CLI authenticated:

```sh
gh api repos/historicalsource/borderzone/contents/COMPILED/spy.z5 \
	-H "Accept: application/vnd.github.raw" > spy.z5
gh api repos/historicalsource/journey/contents/COMPILED/journey.z6 \
	-H "Accept: application/vnd.github.raw" > journey.z6
```

`fetch.sh` checks each file's version, release, serial, and checksum after
downloading, so a truncated or replaced file is obvious immediately.

## Making saves from them

Saves belong here too, under the same names as the committed fixtures —
`<game>-r<release>-<where>.qzl`. A save embeds a difference against its story,
so a save made from an unlicensed story is no more shippable than the story.

Border Zone opens with a chapter prompt, so a scripted save needs to answer it:

```sh
printf '1\nsave\nborderzone-r9-train.qzl\nquit\ny\n' |
	dfrotz -p -m -w 80 spy.z5
```

Journey is version 6 and menu-driven. `dfrotz` cannot run it — the dumb
interface has no graphics, and the game beeps rather than producing a prompt.
Use Gargoyle, which handles V6, and save by hand.

A save from an unlicensed story belongs here rather than in `../gargoyle/`, for
a practical reason as much as a legal one: `interop_test.go` finds a fixture's
story from its name and fails when there is none to find, so a save whose story
cannot be committed has nothing to be read against in a fresh clone.
