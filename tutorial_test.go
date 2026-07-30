// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maloquacious/quetzal"
)

// tutorial.md is held to a stricter standard than the README, and the reason is
// what a tutorial is for. A how-to guide is read by someone who can tell a stale
// snippet from a working one; a tutorial is read by someone who cannot, and who
// has no way to know whether the output that failed to appear was their mistake
// or ours. A lesson that strands its pupil has done worse than teach nothing.
//
// So two tests guard it. TestTutorialProgram runs what the tutorial builds and
// asserts every number the tutorial prints, against the same committed fixtures
// the reader is told to copy. TestTutorialSnippetsAreCompiled checks that the
// code in the markdown is the code that ran.

const (
	tutorialStory = "testdata/stories/zork1-r119-880429.z3"
	tutorialSave  = "testdata/frotz/zork1-r119-kitchen.qzl"
)

// tutorialOutput is what the tutorial tells the reader they will see, joined in
// the order the finished program prints it.
var tutorialOutput = []string{
	"story:  release 119, serial 880429, version 3",
	"        11282 bytes of dynamic memory",
	"save:   release 119, serial 880429, checksum 0xbf44",
	"        PC 0x7590",
	"memory: 11282 bytes, stored as CMem",
	"stack:  5 frames",
	"  frame 0: dummy, 6 word(s) on the evaluation stack",
	"  frame 1: returns to 0x516e, 1 local(s), 0 word(s) on the evaluation stack",
	"  frame 2: returns to 0x56b0, 12 local(s), 0 word(s) on the evaluation stack",
	"  frame 3: returns to 0x5792, 7 local(s), 0 word(s) on the evaluation stack",
	"  frame 4: returns to 0x5a15, 0 local(s), 0 word(s) on the evaluation stack",
	"difference: dynamic memory encoding: CMem vs UMem",
}

// TestTutorialProgram is the tutorial's finished program, with the reader's two
// copied files replaced by the fixtures they are copied from and stdout replaced
// by a buffer. Every line it prints, every file size it quotes, and the byte
// equality it invites the reader to check with cmp are asserted here.
func TestTutorialProgram(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()

	image, err := os.ReadFile(tutorialStory)
	if err != nil {
		t.Fatal(err)
	}
	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&out, "story:  release %d, serial %s, version %d\n",
		story.Release, story.Serial, story.Version)
	fmt.Fprintf(&out, "        %d bytes of dynamic memory\n", len(story.DynamicMemory))

	f, err := os.Open(tutorialSave)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	save, err := quetzal.Read(f, story)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&out, "save:   %s\n", save.Header.Identity())
	fmt.Fprintf(&out, "        PC %#x\n", save.Header.PC)
	fmt.Fprintf(&out, "memory: %d bytes, stored as %s\n",
		len(save.Memory.Data), save.Memory.Encoding)

	fmt.Fprintf(&out, "stack:  %d frames\n", len(save.Frames))
	for i, frame := range save.Frames {
		if frame.IsDummy() {
			fmt.Fprintf(&out, "  frame %d: dummy, %d word(s) on the evaluation stack\n",
				i, len(frame.Evaluation))
			continue
		}
		fmt.Fprintf(&out, "  frame %d: returns to %#x, %d local(s), %d word(s) on the evaluation stack\n",
			i, frame.ReturnPC, len(frame.Locals), len(frame.Evaluation))
	}

	mine := filepath.Join(dir, "mine.qzl")
	w, err := os.Create(mine)
	if err != nil {
		t.Fatal(err)
	}
	if err := quetzal.Write(w, story, save); err != nil {
		t.Fatal(err)
	}
	w.Close()

	plainPath := filepath.Join(dir, "plain.qzl")
	plain, err := os.Create(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := quetzal.Write(plain, story, save,
		quetzal.WithEncoding(quetzal.MemoryUncompressed)); err != nil {
		t.Fatal(err)
	}
	plain.Close()

	g, err := os.Open(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	reread, err := quetzal.Read(g, story)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range quetzal.Compare(save, reread) {
		fmt.Fprintf(&out, "difference: %s\n", d)
	}

	want := strings.Join(tutorialOutput, "\n") + "\n"
	if got := out.String(); got != want {
		t.Errorf("tutorial output changed:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Step 5 tells the reader that cmp reports the two files identical.
	original, err := os.ReadFile(tutorialSave)
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(mine)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, written) {
		t.Errorf("tutorial step 5 claims cmp reports the files identical: "+
			"original %d bytes, rewritten %d bytes, contents differ",
			len(original), len(written))
	}

	// Step 1 quotes the save's size and step 6 quotes both encodings'.
	if len(original) != 434 {
		t.Errorf("tutorial quotes a 434-byte save; fixture is %d bytes", len(original))
	}
	uncompressed, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncompressed) != 11424 {
		t.Errorf("tutorial quotes an 11424-byte uncompressed save; got %d bytes", len(uncompressed))
	}

	// The closing note names the three chunks and two of their sizes.
	h, err := os.Open(tutorialSave)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	file, err := quetzal.Decode(h)
	if err != nil {
		t.Fatal(err)
	}
	wantChunks := []struct {
		id   quetzal.ID
		size int
	}{
		{quetzal.IDIFhd, 13},
		{quetzal.IDCMem, 291},
		{quetzal.IDStks, 92},
	}
	if len(file.Chunks) != len(wantChunks) {
		t.Fatalf("tutorial names %d chunks; container holds %d", len(wantChunks), len(file.Chunks))
	}
	for i, want := range wantChunks {
		if got := file.Chunks[i]; got.ID != want.id || len(got.Data) != want.size {
			t.Errorf("chunk %d: got %s with %d bytes, tutorial says %s with %d",
				i, got.ID, len(got.Data), want.id, want.size)
		}
	}
}

// TestTutorialSnippetsAreCompiled checks that every Go line in tutorial.md
// appears in Example_tutorial, which the compiler checks. Containment runs one
// way, as it does for the README: the example may carry lines the markdown does
// not, but no line of the markdown may go uncompiled.
func TestTutorialSnippetsAreCompiled(t *testing.T) {
	tutorial := readDoc(t, "tutorial.md")
	examples := readDoc(t, "tutorial_test.go")

	compiled := make(map[string]bool)
	for _, line := range strings.Split(examples, "\n") {
		compiled[strings.TrimSpace(line)] = true
	}

	// A program in prose has to open with the two lines an example function
	// cannot contain. Nothing else gets a pass.
	for _, scaffold := range []string{"package main", "func main() {"} {
		compiled[scaffold] = true
	}

	blocks := goBlocks(t, tutorial)
	if len(blocks) == 0 {
		t.Fatal("tutorial.md: found no Go code blocks; has the fence format changed?")
	}

	for i, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !compiled[line] {
				t.Errorf("tutorial.md Go block %d has a line that tutorial_test.go does not compile:\n\t%s",
					i+1, line)
			}
		}
	}
}

// Example_tutorial is the program tutorial.md builds, exactly as the reader
// assembles it, so the compiler checks what they will type. It opens files that
// do not exist here and so carries no Output comment: go test compiles it
// without running it. TestTutorialProgram is what runs the same steps for real.
func Example_tutorial() {
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

	out, err := os.Create("mine.qzl")
	if err != nil {
		log.Fatal(err)
	}
	if err := quetzal.Write(out, story, save); err != nil {
		log.Fatal(err)
	}
	out.Close()

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
}
