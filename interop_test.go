// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/maloquacious/quetzal"
)

// Directories of saves written by other people's interpreters, one per
// interpreter, so that a disagreement points at a program rather than at a
// pile of files. See testdata/README.md.
var saveDirs = []string{"testdata/frotz", "testdata/gargoyle"}

// saveFileName matches a save fixture's name:
//
//	zork1-r119-kitchen.glksave
//
// The game and release identify the story it belongs to, which is how a test
// finds the story without being told. The extension is whichever the
// interpreter that wrote it uses, so that fixtures need no renaming.
var saveFileName = regexp.MustCompile(`^(.+?)-r(\d+)-([a-z0-9-]+)\.(qzl|glksave)$`)

// saveFixture is one committed save and the interpreter that wrote it.
type saveFixture struct {
	// Interpreter is the directory name, which is the interpreter's name.
	Interpreter string

	// Path is where the save lives, and Name is its base name.
	Path string
	Name string
}

// saveFixtures returns every committed save fixture. It fails if there are
// none at all: a suite that silently tests nothing is worse than one that
// fails, since it reports success for work that was never done.
func saveFixtures(t *testing.T) []saveFixture {
	t.Helper()

	var found []saveFixture
	for _, dir := range saveDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) == ".md" {
				continue
			}
			found = append(found, saveFixture{
				Interpreter: filepath.Base(dir),
				Path:        filepath.Join(dir, e.Name()),
				Name:        e.Name(),
			})
		}
	}
	if len(found) == 0 {
		t.Fatalf("no save fixtures in %v; interoperability is untested", saveDirs)
	}
	return found
}

// story returns the story a fixture belongs to, found from the game and
// release its name carries. A save whose name does not follow the convention
// fails here, which is the point of the convention.
func (f saveFixture) story(t *testing.T) quetzal.Story {
	t.Helper()

	m := saveFileName.FindStringSubmatch(f.Name)
	if m == nil {
		t.Fatalf("save fixture %s is not named <game>-r<release>-<where>.<ext>", f.Name)
	}

	matches, err := filepath.Glob(filepath.Join(storiesDir, m[1]+"-r"+m[2]+"-*.z3"))
	if err != nil {
		t.Fatalf("looking for the story of %s: %v", f.Name, err)
	}
	if len(matches) != 1 {
		t.Fatalf("save fixture %s names game %q release %s, which matches %d stories in %s",
			f.Name, m[1], m[2], len(matches), storiesDir)
	}
	return loadStory(t, filepath.Base(matches[0]))
}

// release returns the release number the fixture's name claims.
func (f saveFixture) release(t *testing.T) uint16 {
	t.Helper()

	m := saveFileName.FindStringSubmatch(f.Name)
	if m == nil {
		t.Fatalf("save fixture %s is not named <game>-r<release>-<where>.<ext>", f.Name)
	}
	n, err := strconv.ParseUint(m[2], 10, 16)
	if err != nil {
		t.Fatalf("save fixture %s claims an unusable release number: %v", f.Name, err)
	}
	return uint16(n)
}

// read loads the fixture's bytes.
func (f saveFixture) read(t *testing.T) []byte {
	t.Helper()

	blob, err := os.ReadFile(f.Path)
	if err != nil {
		t.Fatalf("reading %s: %v", f.Path, err)
	}
	return blob
}

// TestInteropReadsForeignSaves is acceptance criterion 8: saves written by
// established interpreters load correctly here. Every committed fixture was
// produced by a program that has never seen this code.
func TestInteropReadsForeignSaves(t *testing.T) {
	for _, fixture := range saveFixtures(t) {
		t.Run(fixture.Interpreter+"/"+fixture.Name, func(t *testing.T) {
			story := fixture.story(t)
			blob := fixture.read(t)

			save, err := quetzal.Read(bytes.NewReader(blob), story)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}

			// The name must describe the file, or a fixture paired with
			// the wrong story would look like a package bug.
			if want := fixture.release(t); save.Header.Release != want {
				t.Errorf("the name claims release %d, but the save says %d",
					want, save.Header.Release)
			}
			if !save.Header.Matches(story) {
				t.Errorf("the save is for %s, but its story is %s",
					save.Header.Identity(), story.Identity())
			}

			// A save reconstructed from a foreign file must be one this
			// package would consider writable, which Read already
			// checks; assert it so the guarantee is visible here.
			if err := save.Validate(story); err != nil {
				t.Errorf("Validate: %v", err)
			}

			if len(save.Memory.Data) != len(story.DynamicMemory) {
				t.Errorf("restored %d bytes of dynamic memory, but the story has %d",
					len(save.Memory.Data), len(story.DynamicMemory))
			}
			if bytes.Equal(save.Memory.Data, story.DynamicMemory) {
				t.Error("restored memory is identical to the story's; a save of a game in progress should differ")
			}

			t.Logf("%s: PC %#x, %s, %d frames, %d additional chunk(s), %d bytes",
				fixture.Interpreter, save.Header.PC, save.Memory.Encoding,
				len(save.Frames), len(save.Chunks), len(blob))
		})
	}
}

// TestInteropRoundTripsForeignSaves checks §18.1 against files this package
// did not write, which is the only place the semantic round trip is tested on
// input it had no hand in producing.
func TestInteropRoundTripsForeignSaves(t *testing.T) {
	for _, fixture := range saveFixtures(t) {
		t.Run(fixture.Interpreter+"/"+fixture.Name, func(t *testing.T) {
			story := fixture.story(t)

			first, err := quetzal.Read(bytes.NewReader(fixture.read(t)), story)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}

			for _, encoding := range []quetzal.MemoryEncoding{
				quetzal.MemoryCompressed,
				quetzal.MemoryUncompressed,
			} {
				t.Run(encoding.String(), func(t *testing.T) {
					var buf bytes.Buffer
					if err := quetzal.Write(&buf, story, first, quetzal.WithEncoding(encoding)); err != nil {
						t.Fatalf("Write: %v", err)
					}

					second, err := quetzal.Read(bytes.NewReader(buf.Bytes()), story)
					if err != nil {
						t.Fatalf("Read(rewritten): %v", err)
					}

					if !headersEqual(second.Header, first.Header) {
						t.Errorf("Header: got %+v, want %+v", second.Header, first.Header)
					}
					if !bytes.Equal(second.Memory.Data, first.Memory.Data) {
						t.Error("dynamic memory did not survive the round trip")
					}
					if !framesEqual(second.Frames, first.Frames) {
						t.Error("the call stack did not survive the round trip")
					}
					if len(second.Chunks) != len(first.Chunks) {
						t.Errorf("kept %d additional chunk(s), want %d",
							len(second.Chunks), len(first.Chunks))
					}
					for i := range first.Chunks {
						if second.Chunks[i].ID != first.Chunks[i].ID ||
							!bytes.Equal(second.Chunks[i].Data, first.Chunks[i].Data) {
							t.Errorf("additional chunk %d changed: got %s, want %s",
								i, second.Chunks[i].ID, first.Chunks[i].ID)
						}
					}
				})
			}
		})
	}
}

// TestInteropDummyFrame is the fixture-backed half of D33. Read requires the
// dummy frame for every version but 6, which is the one place this package
// rejects a file that decodes cleanly, so the evidence that real writers emit
// it belongs in the suite rather than only in spec-deltas.md.
func TestInteropDummyFrame(t *testing.T) {
	for _, fixture := range saveFixtures(t) {
		t.Run(fixture.Interpreter+"/"+fixture.Name, func(t *testing.T) {
			story := fixture.story(t)
			if story.Version == 6 {
				t.Skip("version 6 saves carry no dummy frame")
			}

			save, err := quetzal.Read(bytes.NewReader(fixture.read(t)), story)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(save.Frames) == 0 {
				t.Fatal("no frames at all")
			}
			if !save.Frames[0].IsDummy() {
				t.Errorf("the first frame is not the dummy frame: %+v", save.Frames[0])
			}
		})
	}
}

// TestInteropBocfelSpecifics pins the things Bocfel writes that Frotz does not,
// because each one is a decision this package made in the abstract and this
// file is the evidence the decision was right. Named rather than discovered,
// since the assertions are about this interpreter in particular.
func TestInteropBocfelSpecifics(t *testing.T) {
	const path = "testdata/gargoyle/zork1-r119-kitchen.glksave"

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no Bocfel fixture: %v", err)
	}
	story := loadStory(t, "zork1-r119-880429.z3")

	f, err := quetzal.Decode(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	t.Run("bytes after the FORM are tolerated", func(t *testing.T) {
		// D13. Gargoyle writes eight zero bytes past the end of the FORM,
		// so a reader that demanded end-of-input there would reject every
		// save it produces.
		formEnd := 8 + int(be32(t, blob[4:8]))
		if formEnd >= len(blob) {
			t.Fatalf("this fixture has no trailing bytes; the FORM ends at %d of %d", formEnd, len(blob))
		}
		t.Logf("%d byte(s) follow the FORM", len(blob)-formEnd)
	})

	t.Run("the story reference is not carried into the save", func(t *testing.T) {
		// D34. Bocfel records the absolute path of the story file in an
		// IntD chunk with the s flag set, which standard 7.10 forbids
		// copying to another machine.
		c, ok := f.First(quetzal.IDIntD)
		if !ok {
			t.Fatal("the fixture has no IntD chunk")
		}
		d, err := quetzal.ParseInterpreterData(c.Data)
		if err != nil {
			t.Fatalf("ParseInterpreterData: %v", err)
		}
		if !d.MachineSpecific() {
			t.Errorf("Flags %#02x: the s bit is not set, so this fixture no longer tests the drop rule", d.Flags)
		}
		if d.Copyable() {
			t.Error("Copyable: got true, want false")
		}

		save, err := quetzal.Read(bytes.NewReader(blob), story)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		for _, kept := range save.Chunks {
			if kept.ID == quetzal.IDIntD {
				t.Error("Read carried the machine-specific IntD into the save")
			}
		}
		t.Logf("dropped IntD: os %s, interpreter %q, %d bytes of payload",
			d.OperatingSystem, d.Interpreter, len(d.Data))
	})

	t.Run("an unregistered chunk survives a rewrite", func(t *testing.T) {
		// D26 and D40. Bfhs is Bocfel's own chunk, defined nowhere, and
		// large: the game's scrollback.
		bfhs := quetzal.ID{'B', 'f', 'h', 's'}
		original, ok := f.First(bfhs)
		if !ok {
			t.Fatal("the fixture has no Bfhs chunk")
		}

		save, err := quetzal.Read(bytes.NewReader(blob), story)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}

		var buf bytes.Buffer
		if err := quetzal.Write(&buf, story, save); err != nil {
			t.Fatalf("Write: %v", err)
		}
		rewritten, err := quetzal.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("Decode(rewritten): %v", err)
		}

		got, ok := rewritten.First(bfhs)
		if !ok {
			t.Fatal("the rewritten file lost the Bfhs chunk")
		}
		if !bytes.Equal(got.Data, original.Data) {
			t.Error("the Bfhs payload changed across a rewrite")
		}
		t.Logf("preserved %d bytes of %s", len(got.Data), bfhs)
	})

	t.Run("the annotation names the interpreter", func(t *testing.T) {
		// D29. A real ANNO, which Frotz does not write.
		c, ok := f.First(quetzal.IDANNO)
		if !ok {
			t.Fatal("the fixture has no ANNO chunk")
		}
		for _, b := range c.Data {
			if b < 0x20 || b > 0x7e {
				t.Errorf("ANNO holds byte %#02x, outside the printable ASCII the format requires", b)
			}
		}
		t.Logf("ANNO: %q", c.Data)
	})
}
