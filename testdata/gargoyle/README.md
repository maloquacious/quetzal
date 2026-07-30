# Gargoyle saves

Save files written by [Bocfel](https://github.com/garglk/garglk/tree/master/terps/bocfel)
running under [Gargoyle](https://github.com/garglk/garglk), for Milestone 6.

Frotz is one implementation. Bocfel is a second, written by someone else from
the same standard, and the two agreeing about a file means more than either
agreeing with us. It also writes a much fuller file than Frotz does — see below.

## Naming

```
zork1-r119-kitchen.glksave
      ^^^^ ^^^^^^^
      |    where the game was when it was saved
      release number of the story it belongs to
```

`.glksave` is what Gargoyle produces, so matching it means files come out of
Gargoyle ready to commit with no rename step, the same reason the Frotz saves
are named `.qzl`.

The release number must be the story's real release. Getting it wrong is the
mistake this convention exists to catch, and it is easy to make: `r199` for a
story that is `r119` looks fine until a checksum disagrees.

## Making one

Gargoyle is a GUI application, so unlike `dfrotz` this cannot be scripted.

```sh
brew install --cask gargoyle
```

1. Open Gargoyle. On first launch macOS may block it — right-click the app and
   choose Open, since the cask is not notarized.
2. File → Open, and pick `../stories/zork1-r119-880429.z3`.
3. Play to the position you want. For the Kitchen: `north`, `east`,
   `open window`, `enter`.
4. Type `save` and choose a filename following the convention above.

To check the save, type `restore` in a fresh session and confirm the room is
the one you expect.

## What Bocfel writes that Frotz does not

Worth knowing, because it is why this fixture is valuable:

- **An `ANNO` chunk** naming the interpreter, such as `Interpreter: Bocfel 2.5`.
- **A `Bfhs` chunk**, registered nowhere, holding the game's scrollback as
  16-bit characters. It is large — 2514 bytes in a four-move save — and is the
  package's only real example of an unknown chunk that must survive a rewrite.
- **An `IntD` chunk** recording the absolute path of the story file, with the
  `s` flag set. This is standard 7.12's reference to the original story, and
  standard 7.10 forbids copying it to another machine, so `Read` deliberately
  leaves it out of the `Save`. `Decode` still returns it.
- **Eight bytes after the end of the FORM.** Harmless, and the reason this
  package ignores trailing bytes rather than insisting on end-of-input.

## A caution about the `IntD` path

That chunk contains a real path from the machine the save was made on, which
usually includes a username. Before committing a Gargoyle save, look at what
the path says:

```sh
strings zork1-r119-kitchen.glksave | grep /
```

If it reveals more than you want in public history, make the save with the
story at a neutral location — `/tmp/zork/zork1.z3` and the like — and the
embedded path becomes uninteresting while the fixture keeps all of its value.

## Version coverage

Gargoyle plays version 6 games, which `dfrotz` cannot, and that is worth having:
a V6 save carries no dummy frame, which is the one branch of the stack rules no
other file exercises.

It does not change D43 though, because the constraint is the story files rather
than the interpreter. A V6 save made here belongs in `../local/`, since a
fixture whose story cannot be committed has nothing to be read against in a
fresh clone. See `../../spec-deltas.md`.
