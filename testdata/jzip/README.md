# jzip saves

Save files written by jzip, a third interpreter, and the only one here that can
be told to store dynamic memory uncompressed.

## What jzip is, precisely

jzip 2.1 by John Holder (2000), built from [a custom fork](https://github.com/mdhender/jzip).
The distinction matters when weighing what these fixtures prove:

- **`zork1-r119-kitchen.qzl` is `CMem`, written by jzip's own Quetzal code**,
  which dates from 2000 and has never seen this package. Evidence of the same
  kind as the Frotz and Bocfel fixtures.
- **`zork1-r119-kitchen-umem.qzl` is `UMem`, written by a `-u` flag the fork
  added in 2026.** That code is new, so this file is weaker evidence than the
  others: it shows two independent readings of standard 3.6 agree, not that a
  long-established interpreter does. It is still the only `UMem` file here that
  this package did not write, which is the whole reason it exists — a decoder
  tested only against its own encoder is not tested.

Both were verified to restore in jzip before being committed.

## Making one

```sh
cd testdata/jzip
printf 'north\neast\nopen window\nenter\nsave\nzork1-r119-kitchen.qzl\nquit\ny\n' |
	jzip -l 25 -c 80 ../stories/zork1-r119-880429.z3
```

Add `-u` for uncompressed memory. Unlike Frotz, jzip does not append an
extension, so the name given is the name written. Unlike Gargoyle, it writes no
`IntD`, no `ANNO`, and nothing after the FORM — its files are the minimal three
chunks and nothing else, which makes them the cleanest fixtures here.

To check one, restore it in a fresh run:

```sh
printf 'restore\nzork1-r119-kitchen.qzl\nlook\nquit\ny\n' |
	jzip -l 25 -c 80 ../stories/zork1-r119-880429.z3
```

## ckifzs

The same source tree builds `ckifzs`, the conformance checker standard 9.2
mentions. It is not an interpreter: it reads a save and reports whether the
container is well formed, chunk lengths agree, required chunks are present, and
**the chunks are in the required order**.

```sh
ckifzs some-save.qzl
```

It prints a chunk listing and then either `Save file is valid.` or a count of
errors, and exits 0 or 1 to match — so `ckifzs f >/dev/null || ...` is all a
script needs. Every file this package writes has been through it. Worth
rerunning after any change to the writer, because it checks something no
round-trip test can: a round trip only proves we agree with ourselves.

**Know what its verdict is worth.** Its own usage text says it "does not do
in-depth checking", and measurement bears that out. It reports `Save file is
valid.` for an `IFhd` of 0 or 12 bytes, for a `Stks` whose length is not a whole
number of frames, for a frame claiming more evaluation words than its chunk
holds, and for a `CMem` ending in a dangling zero byte — all of which this
package rejects. So "ckifzs says valid" means the container is sound and the
required chunks are present and ordered; it does not mean the save is
well formed. It is a useful independent check on the parts it does cover, and it
is not a substitute for the test suite.

It is robust, at least: 283 mutated and hand-built hostile inputs through the
sanitizer build produced no crash and no sanitizer report.

**Some of its errors are its own.** Three known defects, filed as
mdhender/jzip#1, #2, and #3, make it reject files that are fine:

- any save carrying an `IntD` chunk, from a one-byte accounting drift;
- any save carrying a chunk it does not recognise, which standard 8.9 says to
  skip;
- any save with bytes after the FORM, which Gargoyle writes on every file.

The practical consequence for this repository: **running `ckifzs` on our rewrite
of a Gargoyle save reports an error that is not ours.** The `Bfhs` scrollback
chunk we correctly preserve is one it will not accept. Use a jzip or Frotz save
as the subject when checking the writer, until those are fixed.

Its verdict on chunk ordering is also the reason D32 stayed strict — see
section 7 of `../../spec-deltas.md`.
