// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

// The examples in README.md, as Go the compiler checks.
//
// specification.md §22 requires that every Go example in the README compile
// against the package and that something automated check it. This file is that
// something: an example function with no "Output:" comment is compiled but not
// run, which is what these need, since they open files that do not exist.
//
// Each function mirrors one section of the README's Usage, in order, and
// TestREADMESnippetsAreCompiled asserts that every line of every README code
// block appears here. An example may carry more than the README shows — a
// snippet in prose can borrow a variable from the section before it, while a
// function has to declare one — so the containment runs one way only.
//
// Editing a README snippet means editing the matching function below. The test
// fails if you do not.

import (
	"fmt"
	"log"
	"os"

	"github.com/maloquacious/quetzal"
)

func Example_inspectASave() {
	f, err := os.Open("save.sav")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	save, err := quetzal.Decode(f)
	if err != nil {
		log.Fatal(err)
	}

	header, err := save.Header()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("story: %s\n", header.Identity())
	fmt.Printf("PC:    %#x\n", header.PC)

	for _, chunk := range save.Chunks {
		fmt.Printf("chunk %s, %d bytes\n", chunk.ID, len(chunk.Data))
	}
}

func Example_checkThatASaveBelongsToAStory() {
	f, err := os.Open("save.sav")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	save, err := quetzal.Decode(f)
	if err != nil {
		log.Fatal(err)
	}

	header, err := save.Header()
	if err != nil {
		log.Fatal(err)
	}

	image, err := os.ReadFile("zork1.z3")
	if err != nil {
		log.Fatal(err)
	}

	story, err := quetzal.ParseStory(image)
	if err != nil {
		log.Fatal(err)
	}

	if err := header.Verify(story); err != nil {
		log.Fatal(err) // wraps quetzal.ErrStoryMismatch
	}
}

func Example_rebuildTheSavedDynamicMemory() {
	save, story := decodedSaveAndStory()

	mem, err := save.Memory(story)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%d bytes of dynamic memory, saved as %s\n", len(mem.Data), mem.Encoding)
}

func Example_walkTheCallStack() {
	save, _ := decodedSaveAndStory()

	frames, err := save.Frames()
	if err != nil {
		log.Fatal(err)
	}

	for i, frame := range frames {
		if frame.IsDummy() {
			fmt.Printf("frame %d: top-level, %d words on the stack\n", i, len(frame.Evaluation))
			continue
		}
		fmt.Printf("frame %d: returns to %#x, %d local(s), %d word(s) on the stack\n",
			i, frame.ReturnPC, len(frame.Locals), len(frame.Evaluation))
	}
}

func Example_readTheTextASaveCarries() {
	save, _ := decodedSaveAndStory()

	for _, note := range save.Annotations() {
		fmt.Printf("annotation: %s\n", note)
	}
	if author, ok := save.Author(); ok {
		fmt.Printf("saved by:   %s\n", author)
	}
}

func Example_changeWhatASaveRecordsAboutItself() {
	save, story := readSaveAndStory()

	out, err := os.Create("save.qzl")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	// Drop any annotation already present and record our own instead.
	var chunks []quetzal.Chunk
	for _, c := range save.Chunks {
		if c.ID != quetzal.IDANNO {
			chunks = append(chunks, c)
		}
	}
	save.Chunks = append(chunks, quetzal.Chunk{
		ID:   quetzal.IDANNO,
		Data: []byte("score 25, 140 moves"),
	})

	if err := quetzal.Write(out, story, save); err != nil {
		log.Fatal(err)
	}
}

func Example_readAndWriteWholeSaves() {
	f, story := openSaveAndStory()
	defer f.Close()

	save, err := quetzal.Read(f, story)
	if err != nil {
		log.Fatal(err)
	}

	// Resume the game from save.Memory.Data, save.Header.PC, and save.Frames,
	// then save it again later.
	out, err := os.Create("save.qzl")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := quetzal.Write(out, story, save); err != nil {
		log.Fatal(err)
	}
}

// The three helpers below stand in for the setup a README section inherits from
// the one before it. They are never called: every function above is an example
// without an "Output:" comment, so the compiler checks it and the test runner
// leaves it alone.

func openSaveAndStory() (*os.File, quetzal.Story) {
	f, err := os.Open("save.sav")
	if err != nil {
		log.Fatal(err)
	}

	image, err := os.ReadFile("zork1.z3")
	if err != nil {
		log.Fatal(err)
	}

	story, err := quetzal.ParseStory(image)
	if err != nil {
		log.Fatal(err)
	}

	return f, story
}

func decodedSaveAndStory() (*quetzal.File, quetzal.Story) {
	f, story := openSaveAndStory()
	defer f.Close()

	save, err := quetzal.Decode(f)
	if err != nil {
		log.Fatal(err)
	}

	return save, story
}

func readSaveAndStory() (*quetzal.Save, quetzal.Story) {
	f, story := openSaveAndStory()
	defer f.Close()

	save, err := quetzal.Read(f, story)
	if err != nil {
		log.Fatal(err)
	}

	return save, story
}
