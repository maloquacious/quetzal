# Frotz saves

Save files written by [Frotz](https://gitlab.com/DavidGriffith/frotz), for
Milestone 6. A save this package produced and then read back proves only that
it agrees with itself; these prove it agrees with an interpreter people
actually use.

## Naming

```
zork1-r119-kitchen.qzl
      ^^^^ ^^^^^^^
      |    where the game was when it was saved
      release number of the story it belongs to
```

The `.qzl` ending is the one Frotz uses. Matching it means the files come out
of Frotz ready to commit, with no renaming step to forget.

The release number ties a save to a story in `../stories/`. A save restored
against the wrong story is the single easiest mistake to make here, and the
one this package is designed to refuse.

## Making one

`dfrotz` is the terminal build of Frotz. On this machine it is at
`/opt/homebrew/bin/dfrotz` (Frotz 2.55). It plays a game the same way a person
would, so a save is made by playing to a spot and typing `save`.

```sh
cd testdata/frotz
printf 'north\neast\nopen window\nenter\nsave\nzork1-r119-kitchen.qzl\nquit\ny\n' |
	dfrotz -p -m -w 80 ../stories/zork1-r119-880429.z3
```

Line by line: the moves that get you where you want to be, then `save`, then
the filename to save under, then `quit` and `y` to confirm.

Three things to know:

- **Frotz adds `.qzl` to any name that does not already end that way.** Ask for
  `kitchen.sav` and you get `kitchen.sav.qzl`. Naming files `.qzl` in the first
  place is why this recipe has no rename step.
- **The file lands in the current directory**, not next to the story, so `cd`
  first.
- **`-p -m -w 80`** turn off formatting codes, MORE prompts, and guessing at
  the terminal width. Without them the output is hard to read and the game may
  wait for a keypress that never comes.

Check the save before trusting it, by restoring it in the same run:

```sh
printf 'restore\nzork1-r119-kitchen.qzl\nlook\nquit\ny\n' |
	dfrotz -p -m -w 80 ../stories/zork1-r119-880429.z3
```

If `look` describes the room you expected, the save is good.

## What to collect

§19 of `specification.md` lists the fixtures interoperability testing wants,
and §6 of `spec-deltas.md` maps each one to the decisions it would test. The
gaps worth a real file most are a version 1 or 2 game (delta D27, the one
likely to be a genuine bug), a version 6 game, and a save whose chunks are in
a non-standard order.

Frotz saves compressed memory by default, so a `UMem` fixture needs a different
interpreter or a hand-built file.

Section 7 of `spec-deltas.md` records what one Frotz save has already shown,
and — more usefully — what it did not reach.
