# Tutorial: read your first save

This is a lesson, not a manual. We are going to take one real saved game — a
position in Zork I written by Frotz, an interpreter that has nothing to do with
this package — and pull it apart until we can say exactly where the player was
standing and what the game was in the middle of doing. Then we will write it
back out and prove we did not lose anything.

You do not need to know anything about Quetzal or the Z-machine to follow along.
You will need Go 1.26 or later, a terminal, and about fifteen minutes.

When you are done, the how-to guides in [README.md](README.md) will make sense,
because you will have handled every piece they refer to.

## Step 1: get set up

We need two files: a story and a save. Both are committed to this repository, so
clone it and work in a directory beside it.

```sh
git clone https://github.com/maloquacious/quetzal.git
mkdir first-save
cd first-save
```

Copy in the two files, giving them short names so the code stays readable:

```sh
cp ../quetzal/testdata/stories/zork1-r119-880429.z3 zork1.z3
cp ../quetzal/testdata/frotz/zork1-r119-kitchen.qzl kitchen.qzl
```

Set up a module and add the package:

```sh
go mod init example/first-save
go get github.com/maloquacious/quetzal
```

Look at what you just copied:

```sh
ls -l zork1.z3 kitchen.qzl
```

The story is about 87,000 bytes. The save is 434.

Stop and notice that, because it is the whole reason this package has the shape
it does. Four hundred and thirty-four bytes cannot possibly describe the state of
a game. The save is not a snapshot; it is a *difference* against the story it
came from. That is why nothing here can read a save without also being handed its
story — and why we copied two files instead of one.

## Step 2: load the story

Create `main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/maloquacious/quetzal"
)

func main() {
	image, err := os.ReadFile("zork1.z3")
	if err != nil {
		log.Fatal(err)
	}

	story, err := quetzal.ParseStory(image)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("story:  release %d, serial %s, version %d\n",
		story.Release, story.Serial, story.Version)
	fmt.Printf("        %d bytes of dynamic memory\n", len(story.DynamicMemory))
}
```

Run it:

```sh
go run .
```

```
story:  release 119, serial 880429, version 3
        11282 bytes of dynamic memory
```

Three things just happened worth naming.

`ParseStory` read the story's header and kept only what saving and restoring
need. It is not a Z-machine and it has not begun to run anything.

The release number and serial date are how a story identifies itself. Zork I
release 119 was built on 29 April 1988, and the serial is that date. A save
records these so that restoring it into the wrong game fails loudly instead of
producing nonsense.

And 11,282 is the size of *dynamic memory* — the part of the story the game is
allowed to change as you play. Everything above it is fixed and never needs
saving. Our 434-byte save is a difference against those 11,282 bytes.

## Step 3: read the save

Now open the save and hand it the story. Add this to the end of `main`:

```go
	f, err := os.Open("kitchen.qzl")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	save, err := quetzal.Read(f, story)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("save:   %s\n", save.Header.Identity())
	fmt.Printf("        PC %#x\n", save.Header.PC)
	fmt.Printf("memory: %d bytes, stored as %s\n",
		len(save.Memory.Data), save.Memory.Encoding)
```

Run it again:

```
story:  release 119, serial 880429, version 3
        11282 bytes of dynamic memory
save:   release 119, serial 880429, checksum 0xbf44
        PC 0x7590
memory: 11282 bytes, stored as CMem
```

The save's identity matches the story's, which is what let `Read` succeed. Had
you passed Zork II, it would have failed with an error wrapping
`quetzal.ErrStoryMismatch` rather than handing you a plausible-looking wreck.

`PC` is the program counter: the exact instruction the interpreter was about to
execute when the player typed `save`. It is where the game resumes.

And there is the difference made whole. `Memory.Data` is 11,282 bytes — the full
size of dynamic memory, not the 434 the file took — because `Read` rebuilt it by
applying the save's stored difference to the story you supplied. `Encoding` says
`CMem`, which is the compressed form: the format's name for that difference.

## Step 4: look at the call stack

A save records where the game was in its own code, not just where the player was
standing. Add this:

```go
	fmt.Printf("stack:  %d frames\n", len(save.Frames))
	for i, frame := range save.Frames {
		if frame.IsDummy() {
			fmt.Printf("  frame %d: dummy, %d word(s) on the evaluation stack\n",
				i, len(frame.Evaluation))
			continue
		}
		fmt.Printf("  frame %d: returns to %#x, %d local(s), %d word(s) on the evaluation stack\n",
			i, frame.ReturnPC, len(frame.Locals), len(frame.Evaluation))
	}
```

```
stack:  5 frames
  frame 0: dummy, 6 word(s) on the evaluation stack
  frame 1: returns to 0x516e, 1 local(s), 0 word(s) on the evaluation stack
  frame 2: returns to 0x56b0, 12 local(s), 0 word(s) on the evaluation stack
  frame 3: returns to 0x5792, 7 local(s), 0 word(s) on the evaluation stack
  frame 4: returns to 0x5a15, 0 local(s), 0 word(s) on the evaluation stack
```

Frames run oldest first, so frame 4 is the routine that was executing when the
game was saved and frame 0 is the outermost.

Frame 0 is a *dummy* frame, and it is worth a moment. In Z-machine versions
below 6 the game's outermost code is not a routine call, so there is no real
frame to record for it — but there may still be values on the evaluation stack
belonging to it, as there are here, six of them. The format's answer is a
placeholder frame that owns those values and returns nowhere. Every save you
meet from a version 1 through 5 story will have one.

The other four are ordinary calls, each with its own local variables and its own
return address.

## Step 5: write it back out

We have taken the save apart. Now put it back together. Add this:

```go
	out, err := os.Create("mine.qzl")
	if err != nil {
		log.Fatal(err)
	}
	if err := quetzal.Write(out, story, save); err != nil {
		log.Fatal(err)
	}
	out.Close()
```

Then compare the two files in your shell:

```sh
go run .
cmp kitchen.qzl mine.qzl && echo identical
```

```
identical
```

Byte for byte the same file Frotz wrote.

Enjoy that, but do not rely on it. What this package promises is that a save
survives the trip *semantically* — same story identity, same dynamic memory, same
program counter, same frames — not that the bytes come back the same. They
matched here because Frotz made the same encoding choice we did. The next step
breaks the tie deliberately, so you can see what the promise is actually worth.

## Step 6: change how it is stored, not what it says

The format lets a writer store dynamic memory either as a difference (`CMem`) or
in full (`UMem`). Ask for the second. Add this:

```go
	plain, err := os.Create("plain.qzl")
	if err != nil {
		log.Fatal(err)
	}
	if err := quetzal.Write(plain, story, save,
		quetzal.WithEncoding(quetzal.MemoryUncompressed)); err != nil {
		log.Fatal(err)
	}
	plain.Close()

	g, err := os.Open("plain.qzl")
	if err != nil {
		log.Fatal(err)
	}
	defer g.Close()

	reread, err := quetzal.Read(g, story)
	if err != nil {
		log.Fatal(err)
	}

	for _, d := range quetzal.Compare(save, reread) {
		fmt.Printf("difference: %s\n", d)
	}
```

```
difference: dynamic memory encoding: CMem vs UMem
```

Check the sizes:

```sh
ls -l kitchen.qzl plain.qzl
```

434 bytes against 11,424. Twenty-six times the file, and `Compare` — which walks
both saves and reports everything it finds — has exactly one thing to say about
them, which is that they are stored differently. Not one byte of dynamic memory
differs. Not the program counter, not a frame, not a local.

That is the semantic round trip, and you have now watched it hold across a
change of encoding. If you would rather not hear about the encoding at all,
`quetzal.IgnoreMemoryEncoding()` passed to `Compare` silences that last
difference and the comparison reports nothing whatsoever.

## What you did

You loaded a story, read another interpreter's save against it, found the
position and the call stack inside it, wrote it back out unchanged, wrote it
again in a different encoding, and proved the two say the same thing.

That is the whole shape of this package. Everything else is detail:

- **[README.md](README.md)** has the how-to guides — checking a save belongs to
  a story, editing what a save records, comparing against another interpreter.
  They will read easily now.
- **`go doc github.com/maloquacious/quetzal`** is the reference. Start with the
  package overview, which explains why `Parse`, `Decode`, `Validate`, and
  `Compare` mean what they do.
- **[specification.md](specification.md)** is what this package promises, and
  **[spec-deltas.md](spec-deltas.md)** is where every deliberate departure from
  Quetzal 1.4 is argued out.

One loose end, if you want it. `Read` needed the story. `Decode` does not — it
parses the container and hands you the raw chunks, which is enough to inspect a
save whose story you do not have. Try it on `kitchen.qzl` and you will find the
three chunks every save carries: `IFhd`, 13 bytes of story identity; `CMem`, the
291-byte difference; and `Stks`, 92 bytes holding those five frames.
