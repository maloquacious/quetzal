// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"bytes"
	"fmt"
)

// Comparison is not part of the Quetzal format. It exists so that a difference
// between a save this package wrote and one another interpreter wrote is
// something a test can print rather than something a person has to find in a
// byte dump, and specification.md §5.7 places it outside that document's scope
// for exactly that reason. The design is recorded in
// https://github.com/maloquacious/quetzal/issues/1 and the behavior is specified
// by the doc comments below.
//
// Two properties shape everything here.
//
// Comparison cannot fail. Two values of one kind always differ in some
// enumerable way or not at all, so there is no error to return and no judgment
// to pass: neither side is the expected one, and an empty result means only that
// the two agree under the options given.
//
// The resource limits of §16 do not apply, because §16.2 bounds reading and
// nothing here reads. Both sides are values the caller already holds. What keeps
// the result readable instead is that runs of differing memory bytes are
// coalesced into one Difference each, so two saves whose dynamic memory has
// nothing in common report a handful of differences rather than one per byte.

// noIndex marks a Difference field that the kind of difference gives no meaning
// to: the frame of something outside the call stack, or the offset of something
// that is not a position within a larger value.
const noIndex = -1

// DifferenceKind names what differs, and with it the type held in
// Difference.A and Difference.B. The table in Difference gives that type for
// each kind.
type DifferenceKind uint8

// The kinds of difference Compare and CompareFiles report. They are grouped as
// a save is: story identification, dynamic memory, the call stack, and the
// remaining chunks.
const (
	// Differences in the IFhd chunk.
	DiffRelease        DifferenceKind = iota + 1 // release number
	DiffSerial                                   // serial number
	DiffChecksum                                 // story checksum
	DiffProgramCounter                           // saved program counter
	DiffHeaderExtra                              // bytes beyond the 13 Quetzal defines

	// Differences in dynamic memory.
	DiffMemoryEncoding // stored as CMem on one side and UMem on the other
	DiffMemorySize     // the two hold different amounts of dynamic memory
	DiffMemoryBytes    // a run of bytes that differ

	// Differences in the call stack.
	DiffFrameCount      // the stacks are of different depths
	DiffReturnPC        // a frame returns to a different address
	DiffDiscardResult   // a frame's p bit differs
	DiffResultVariable  // a frame stores its result in a different variable
	DiffArguments       // a frame's argument-supplied mask differs
	DiffLocalCount      // a frame has a different number of locals
	DiffLocalValue      // a frame holds a different value in one local
	DiffEvaluationDepth // a frame has a different amount of evaluation stack
	DiffEvaluationValue // a frame holds a different value in one stack word

	// Differences in the remaining chunks.
	DiffChunkCount // a different number of chunks, of one identifier or in total
	DiffChunkID    // a different chunk identifier at the same position
	DiffChunkData  // a chunk with a different payload
)

// String names the kind of difference. The name is a noun phrase, so that it
// reads as the subject of a sentence a caller is building for itself; Difference
// itself has a String that produces the whole sentence.
func (k DifferenceKind) String() string {
	switch k {
	case DiffRelease:
		return "release"
	case DiffSerial:
		return "serial"
	case DiffChecksum:
		return "checksum"
	case DiffProgramCounter:
		return "program counter"
	case DiffHeaderExtra:
		return "IFhd extra bytes"
	case DiffMemoryEncoding:
		return "memory encoding"
	case DiffMemorySize:
		return "memory size"
	case DiffMemoryBytes:
		return "memory bytes"
	case DiffFrameCount:
		return "frame count"
	case DiffReturnPC:
		return "return program counter"
	case DiffDiscardResult:
		return "discard-result flag"
	case DiffResultVariable:
		return "result variable"
	case DiffArguments:
		return "arguments mask"
	case DiffLocalCount:
		return "local count"
	case DiffLocalValue:
		return "local variable"
	case DiffEvaluationDepth:
		return "evaluation stack depth"
	case DiffEvaluationValue:
		return "evaluation stack word"
	case DiffChunkCount:
		return "chunk count"
	case DiffChunkID:
		return "chunk identifier"
	case DiffChunkData:
		return "chunk payload"
	default:
		return fmt.Sprintf("DifferenceKind(%d)", uint8(k))
	}
}

// Difference is one way in which two saves, or two containers, differ.
//
// A and B hold the differing values from the first and second argument to
// Compare, in that order. Both are always set: a value present on one side and
// absent on the other is reported as a difference in a count — a chunk
// identifier that appears twice on one side and not at all on the other is
// DiffChunkCount with A of 2 and B of 0 — so neither field is ever nil to mean
// absence, and a caller need not distinguish absent from zero.
//
// The type they hold follows from Kind:
//
//	DiffRelease, DiffChecksum                        uint16
//	DiffSerial                                       Serial
//	DiffProgramCounter, DiffReturnPC                 uint32
//	DiffHeaderExtra, DiffMemoryBytes, DiffChunkData  []byte
//	DiffMemoryEncoding                               MemoryEncoding
//	DiffDiscardResult                                bool
//	DiffResultVariable, DiffArguments                byte
//	DiffLocalValue, DiffEvaluationValue              uint16
//	DiffChunkID                                      ID
//	every count and size                             int
//
// Any []byte is a copy. Nothing a Difference holds aliases the saves it came
// from, so a caller may keep the result and mutate the inputs.
type Difference struct {
	// Kind is what differs.
	Kind DifferenceKind

	// Frame is the index of the stack frame the difference belongs to,
	// counting from zero at the oldest frame, or noIndex for a difference
	// outside the call stack.
	Frame int

	// Offset locates the difference within what Kind names: the byte address
	// in dynamic memory, the index of a local variable or evaluation-stack
	// word, or the position of a chunk. It is negative where the kind names a
	// whole value rather than a position within one.
	Offset int

	// ID names the chunk a chunk difference belongs to. It is the zero ID
	// where the difference is not about a chunk, and also on the
	// DiffChunkCount that CompareFiles reports for the total number of
	// chunks — four zero bytes are not a valid chunk identifier, so the zero
	// value is unambiguous.
	ID ID

	// A and B are the differing values, from the first and second argument
	// respectively. See the type table above.
	A, B any
}

// String describes the difference in one line, in the form "what: A vs B".
//
// It is meant to be printed by a failing test, so it favors being readable over
// being parsed: values appear in whichever base the format stores them in, and a
// long run of differing memory is abbreviated. A caller that wants the values
// themselves takes them from A and B.
func (d Difference) String() string {
	if d.Frame >= 0 {
		return fmt.Sprintf("frame %d: %s", d.Frame, d.describe())
	}
	return d.describe()
}

// describe renders the difference without the frame it belongs to, which String
// supplies.
func (d Difference) describe() string {
	switch d.Kind {
	case DiffRelease:
		return fmt.Sprintf("release: %v vs %v", d.A, d.B)
	case DiffSerial:
		return fmt.Sprintf("serial: %v vs %v", d.A, d.B)
	case DiffChecksum:
		return fmt.Sprintf("checksum: %#04x vs %#04x", d.A, d.B)
	case DiffProgramCounter:
		return fmt.Sprintf("program counter: %#x vs %#x", d.A, d.B)
	case DiffHeaderExtra:
		return fmt.Sprintf("%s bytes beyond the first %d: %s vs %s",
			IDIFhd, ifhdSize, byteCount(d.A), byteCount(d.B))
	case DiffMemoryEncoding:
		return fmt.Sprintf("dynamic memory encoding: %v vs %v", d.A, d.B)
	case DiffMemorySize:
		return fmt.Sprintf("dynamic memory: %v byte(s) vs %v", d.A, d.B)
	case DiffMemoryBytes:
		return fmt.Sprintf("dynamic memory at %#04x: %s vs %s", d.Offset, hexRun(d.A), hexRun(d.B))
	case DiffFrameCount:
		return fmt.Sprintf("call stack: %v frame(s) vs %v", d.A, d.B)
	case DiffReturnPC:
		return fmt.Sprintf("return program counter: %#x vs %#x", d.A, d.B)
	case DiffDiscardResult:
		return fmt.Sprintf("discard-result flag: %v vs %v", d.A, d.B)
	case DiffResultVariable:
		return fmt.Sprintf("result variable: %v vs %v", d.A, d.B)
	case DiffArguments:
		return fmt.Sprintf("arguments mask: %#02x vs %#02x", d.A, d.B)
	case DiffLocalCount:
		return fmt.Sprintf("%v local(s) vs %v", d.A, d.B)
	case DiffLocalValue:
		// Locals[0] is local 1, so the local a caller would name is one
		// past the index this difference carries.
		return fmt.Sprintf("local %d: %#04x vs %#04x", d.Offset+1, d.A, d.B)
	case DiffEvaluationDepth:
		return fmt.Sprintf("%v evaluation word(s) vs %v", d.A, d.B)
	case DiffEvaluationValue:
		return fmt.Sprintf("evaluation word %d: %#04x vs %#04x", d.Offset, d.A, d.B)
	case DiffChunkCount:
		if d.ID == (ID{}) {
			return fmt.Sprintf("chunks: %v vs %v", d.A, d.B)
		}
		return fmt.Sprintf("%s chunks: %v vs %v", d.ID, d.A, d.B)
	case DiffChunkID:
		return fmt.Sprintf("chunk %d: %v vs %v", d.Offset, d.A, d.B)
	case DiffChunkData:
		return fmt.Sprintf("%s chunk %d: payloads differ, %s vs %s",
			d.ID, d.Offset, byteCount(d.A), byteCount(d.B))
	default:
		// A Difference a caller built by hand, with a kind this package
		// does not define. Reporting it plainly is better than saying
		// nothing, and better than panicking on input that is merely odd.
		return fmt.Sprintf("%s: %v vs %v", d.Kind, d.A, d.B)
	}
}

// CompareOption configures a comparison.
//
// Every option is named for what it disregards, and disregarding is all an
// option here can do: none of them can turn agreement into a difference. A
// comparison run with every option is therefore the most forgiving one
// available, and one run with none is exact.
type CompareOption func(*compareConfig)

// compareConfig holds the settings a CompareOption may adjust.
type compareConfig struct {
	// ignoreMemoryEncoding drops the CMem-versus-UMem difference.
	ignoreMemoryEncoding bool

	// ignoredMemory are the address ranges within dynamic memory not to
	// compare, in the order the options named them.
	ignoredMemory []memoryRange

	// ignoredChunks are the chunk identifiers not to compare. A nil map
	// ignores nothing, which is what a comparison with no options wants.
	ignoredChunks map[ID]bool
}

// memoryRange is a half-open range of dynamic memory addresses.
type memoryRange struct {
	start, end int
}

// newCompareConfig applies the options given.
func newCompareConfig(opts []CompareOption) compareConfig {
	var cfg compareConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// ignored reports whether the given address of dynamic memory is one no option
// asked to compare.
func (cfg compareConfig) ignored(addr int) bool {
	for _, r := range cfg.ignoredMemory {
		if addr >= r.start && addr < r.end {
			return true
		}
	}
	return false
}

// IgnoreMemoryEncoding disregards whether dynamic memory was stored compressed
// or uncompressed.
//
// The encoding is a writer's choice and not part of the state a save records:
// the same memory stored either way restores the same game, which is what §18.1
// means by round-tripping semantically rather than byte for byte. Two
// interpreters comparing notes about a saved position almost never mean to
// compare this, and one of them writing UMem while the other writes CMem is the
// most likely single difference between any two conforming writers.
func IgnoreMemoryEncoding() CompareOption {
	return func(cfg *compareConfig) { cfg.ignoreMemoryEncoding = true }
}

// IgnoreMemoryRange disregards dynamic memory from start up to but not
// including end.
//
// A range that is empty or inverted disregards nothing, and one reaching outside
// dynamic memory disregards only the part that lies inside, so no combination of
// bounds is an error. Ranges accumulate: passing the option twice disregards
// both.
//
// An ignored address also ends whatever run of differing bytes precedes it, so
// disregarding a range in the middle of a difference splits the DiffMemoryBytes
// that would otherwise have covered it into two.
func IgnoreMemoryRange(start, end int) CompareOption {
	return func(cfg *compareConfig) {
		cfg.ignoredMemory = append(cfg.ignoredMemory, memoryRange{start: start, end: end})
	}
}

// interpreterHeader is the part of the Z-machine header that the interpreter
// writes rather than the game, as ranges within dynamic memory. Standard 11.1
// marks each of these fields as one the interpreter fills in, in the Version
// that introduced it; this set is the union across Versions and so is wider
// than any single Version needs. See IgnoreInterpreterHeader.
var interpreterHeader = []memoryRange{
	{0x01, 0x02}, // Flags 1: what the interpreter can provide
	{0x10, 0x12}, // Flags 2: the requests it granted
	{0x1e, 0x28}, // interpreter number and version, screen size, font size
	{0x2c, 0x2e}, // default background and foreground colour
	{0x30, 0x34}, // width of text sent to stream 3, standard revision number
}

// IgnoreInterpreterHeader disregards the fields of the Z-machine header that the
// interpreter writes rather than the game.
//
// This is the option a cross-interpreter comparison almost certainly wants, and
// the reason is a detail of where the header lives. Dynamic memory runs from
// address zero, so the whole 64-byte header sits inside it and is saved with it —
// including every field the interpreter filled in for itself. Two interpreters
// will differ on the interpreter number and version, the screen size in lines
// and columns and again in units, the font size, the default colours, the width
// of text sent to output stream 3, and the standard revision they claim. None of
// that describes the saved position. Without this option those fields are the
// first dozen differences reported between any two interpreters, every time, and
// they bury the one difference that was worth looking for.
//
// The ranges disregarded are $01, $10 to $11, $1E to $27, $2C to $2D, and $30 to
// $33. That set is the union across Versions, not the set that applies to the
// Version of the save being compared: Standard 11.1 introduces $1E to $21 at
// Version 4, $22 to $27 and $2C to $2D at Version 5, and $30 to $31 at Version 6.
// On a save below Version 4 those ten bytes are ordinary dynamic memory rather
// than fields the interpreter owns, and this option disregards them anyway. In
// practice they are zero on both sides of such a comparison, since neither the
// game nor the interpreter writes them, so nothing observable is hidden; the
// option stays declarative rather than making its effect depend on the values it
// is comparing.
//
// Two of those ranges are wider than they strictly need to be. Flags 1 and
// Flags 2 each mix bits the game sets with bits the interpreter sets, and
// disregarding an address is byte-granular, so a game-written flag in the same
// byte as an interpreter-written one goes uncompared with it. Reporting those
// bits would mean reporting the interpreter's bits alongside them, which is the
// noise this exists to remove; a caller that needs one of them can compare
// Memory.Data itself.
func IgnoreInterpreterHeader() CompareOption {
	return func(cfg *compareConfig) {
		cfg.ignoredMemory = append(cfg.ignoredMemory, interpreterHeader...)
	}
}

// IgnoreChunks disregards every chunk with one of the given identifiers.
//
// ANNO and IntD are the usual arguments. Each interpreter writes its own
// annotations and its own interpreter data, and neither is state the game
// depends on: the format is explicit that an interpreter must not rely on the
// text chunks being present (7.6), and IntD holds what one interpreter needs and
// others need not understand.
//
// CompareFiles drops these chunks before comparing anything, so that ignoring a
// chunk one file carries and the other does not lines up the chunks that follow
// it rather than reporting every one of them as displaced.
func IgnoreChunks(ids ...ID) CompareOption {
	return func(cfg *compareConfig) {
		if cfg.ignoredChunks == nil {
			cfg.ignoredChunks = make(map[ID]bool, len(ids))
		}
		for _, id := range ids {
			cfg.ignoredChunks[id] = true
		}
	}
}

// Compare reports every way in which two saves differ, in a fixed order: story
// identification, dynamic memory, the call stack, then the remaining chunks. An
// empty result means the two agree under the options given.
//
// The saves need not belong to the same story. Comparing saves of two different
// stories is how a caller finds out that is what it has, and the release, serial,
// and checksum differences say so; no story image is needed to answer the
// question, and none is asked for.
//
// Differences are reported at the finest granularity that stays readable. Frames
// are compared oldest first, which is the order they are stored in and the order
// in which two stacks of different depths share a prefix, so a difference deep in
// a stack is reported against the frame it belongs to rather than displacing
// every frame after it. Runs of differing memory bytes are coalesced, so memory
// that differs everywhere reports one difference rather than thousands.
//
// One byte is deliberately not compared. A frame that discards its result gives
// no meaning to its result variable, and this package zeroes that byte on write
// while preserving whatever it read (D16), so two saves of one position can
// disagree there without describing different states. The variable is compared
// only when neither frame discards its result; when they disagree about
// discarding, DiffDiscardResult reports that instead.
//
// A nil save is compared as an empty one, which reports a difference against
// every field the other holds rather than panicking. Neither save is modified,
// and nothing in the result aliases either of them.
func Compare(a, b *Save, opts ...CompareOption) []Difference {
	cfg := newCompareConfig(opts)
	if a == nil {
		a = &Save{}
	}
	if b == nil {
		b = &Save{}
	}

	var diffs []Difference
	diffs = appendHeaderDifferences(diffs, a.Header, b.Header)
	diffs = cfg.appendMemoryDifferences(diffs, a.Memory, b.Memory)
	diffs = appendFrameDifferences(diffs, a.Frames, b.Frames)
	return cfg.appendChunkDifferences(diffs, a.Chunks, b.Chunks)
}

// CompareFiles reports every way in which two containers differ, comparing
// chunks by position: how many there are, then the identifier and payload of
// each.
//
// Where Compare asks whether two saves record the same state, this asks whether
// two files say it the same way. Chunk order is a difference here and not there,
// because order is what a container has and a save does not: Quetzal requires
// IFhd before the memory and stack chunks, and a File is the layer that can
// still see whether a writer obeyed. Nothing is interpreted, so no story is
// needed and a container that is not a valid save compares as readily as one
// that is.
//
// Only IgnoreChunks has any effect. The memory options describe dynamic memory,
// and a container holds an encoded payload rather than dynamic memory — a CMem
// payload compared against a UMem payload differs along its whole length, and no
// range of addresses within either one means what the option's argument would
// suggest. Compare is the layer where those options belong.
//
// A nil file is compared as an empty one. Neither file is modified, and nothing
// in the result aliases either of them.
func CompareFiles(a, b *File, opts ...CompareOption) []Difference {
	cfg := newCompareConfig(opts)
	if a == nil {
		a = &File{}
	}
	if b == nil {
		b = &File{}
	}

	x, y := cfg.keep(a.Chunks), cfg.keep(b.Chunks)

	var diffs []Difference
	if len(x) != len(y) {
		diffs = append(diffs, difference(DiffChunkCount, len(x), len(y)))
	}
	for i := 0; i < min(len(x), len(y)); i++ {
		if x[i].ID != y[i].ID {
			diffs = append(diffs, Difference{
				Kind: DiffChunkID, Frame: noIndex, Offset: i,
				A: x[i].ID, B: y[i].ID,
			})
			// The payloads of two chunks that are not the same chunk have
			// nothing to say to each other, and reporting that they differ
			// would only repeat what the identifiers already said.
			continue
		}
		if !bytes.Equal(x[i].Data, y[i].Data) {
			diffs = append(diffs, Difference{
				Kind: DiffChunkData, Frame: noIndex, Offset: i, ID: x[i].ID,
				A: clone(x[i].Data), B: clone(y[i].Data),
			})
		}
	}
	return diffs
}

// keep returns the chunks that no option asked to disregard, in order.
func (cfg compareConfig) keep(chunks []Chunk) []Chunk {
	kept := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		if !cfg.ignoredChunks[c.ID] {
			kept = append(kept, c)
		}
	}
	return kept
}

// appendHeaderDifferences compares two saves' story identification.
func appendHeaderDifferences(diffs []Difference, a, b Header) []Difference {
	if a.Release != b.Release {
		diffs = append(diffs, difference(DiffRelease, a.Release, b.Release))
	}
	if a.Serial != b.Serial {
		diffs = append(diffs, difference(DiffSerial, a.Serial, b.Serial))
	}
	if a.Checksum != b.Checksum {
		diffs = append(diffs, difference(DiffChecksum, a.Checksum, b.Checksum))
	}
	if a.PC != b.PC {
		diffs = append(diffs, difference(DiffProgramCounter, a.PC, b.PC))
	}
	// An absent Extra and an empty one are the same statement about the file:
	// that it carried nothing beyond the 13 bytes Quetzal defines.
	if !bytes.Equal(a.Extra, b.Extra) {
		diffs = append(diffs, difference(DiffHeaderExtra, clone(a.Extra), clone(b.Extra)))
	}
	return diffs
}

// appendMemoryDifferences compares two saves' dynamic memory, coalescing
// differing bytes into runs.
func (cfg compareConfig) appendMemoryDifferences(diffs []Difference, a, b Memory) []Difference {
	if !cfg.ignoreMemoryEncoding && a.Encoding != b.Encoding {
		diffs = append(diffs, difference(DiffMemoryEncoding, a.Encoding, b.Encoding))
	}
	if len(a.Data) != len(b.Data) {
		diffs = append(diffs, difference(DiffMemorySize, len(a.Data), len(b.Data)))
	}

	// Only the addresses both sides have can be compared. A difference in
	// length is already reported, and comparing past the shorter of the two
	// would report the longer one's remaining bytes as differing from nothing.
	n := min(len(a.Data), len(b.Data))
	for i := 0; i < n; {
		if a.Data[i] == b.Data[i] || cfg.ignored(i) {
			i++
			continue
		}
		start := i
		for i < n && a.Data[i] != b.Data[i] && !cfg.ignored(i) {
			i++
		}
		diffs = append(diffs, Difference{
			Kind: DiffMemoryBytes, Frame: noIndex, Offset: start,
			A: clone(a.Data[start:i]), B: clone(b.Data[start:i]),
		})
	}
	return diffs
}

// appendFrameDifferences compares two call stacks, oldest frame first.
func appendFrameDifferences(diffs []Difference, a, b []Frame) []Difference {
	if len(a) != len(b) {
		diffs = append(diffs, difference(DiffFrameCount, len(a), len(b)))
	}
	for i := 0; i < min(len(a), len(b)); i++ {
		diffs = appendOneFrameDifferences(diffs, i, a[i], b[i])
	}
	return diffs
}

// appendOneFrameDifferences compares the frames at one depth of two call stacks.
func appendOneFrameDifferences(diffs []Difference, index int, a, b Frame) []Difference {
	// at builds a difference belonging to this frame, at a position within
	// one of its fields or at none.
	at := func(kind DifferenceKind, offset int, x, y any) Difference {
		return Difference{Kind: kind, Frame: index, Offset: offset, A: x, B: y}
	}

	if a.ReturnPC != b.ReturnPC {
		diffs = append(diffs, at(DiffReturnPC, noIndex, a.ReturnPC, b.ReturnPC))
	}
	if a.DiscardResult != b.DiscardResult {
		diffs = append(diffs, at(DiffDiscardResult, noIndex, a.DiscardResult, b.DiscardResult))
	}
	// The result variable of a frame that discards its result carries no
	// meaning, so two saves of one position may hold anything there and still
	// describe the same state. See Compare and D16.
	if !a.DiscardResult && !b.DiscardResult && a.ResultVariable != b.ResultVariable {
		diffs = append(diffs, at(DiffResultVariable, noIndex, a.ResultVariable, b.ResultVariable))
	}
	if a.Arguments != b.Arguments {
		diffs = append(diffs, at(DiffArguments, noIndex, a.Arguments, b.Arguments))
	}

	if len(a.Locals) != len(b.Locals) {
		diffs = append(diffs, at(DiffLocalCount, noIndex, len(a.Locals), len(b.Locals)))
	}
	for i := 0; i < min(len(a.Locals), len(b.Locals)); i++ {
		if a.Locals[i] != b.Locals[i] {
			diffs = append(diffs, at(DiffLocalValue, i, a.Locals[i], b.Locals[i]))
		}
	}

	if len(a.Evaluation) != len(b.Evaluation) {
		diffs = append(diffs, at(DiffEvaluationDepth, noIndex, len(a.Evaluation), len(b.Evaluation)))
	}
	for i := 0; i < min(len(a.Evaluation), len(b.Evaluation)); i++ {
		if a.Evaluation[i] != b.Evaluation[i] {
			diffs = append(diffs, at(DiffEvaluationValue, i, a.Evaluation[i], b.Evaluation[i]))
		}
	}
	return diffs
}

// appendChunkDifferences compares the chunks two saves carry beyond the ones
// their interpreted fields describe.
//
// The chunks are grouped by identifier rather than compared by position, which
// is the difference between this and CompareFiles. A save's chunk order is
// preserved but says little: Quetzal fixes the order of the chunks it requires
// and leaves these to follow in whatever order they arrived, so two saves
// carrying the same annotation and the same interpreter data in the opposite
// order are not describing different positions. What matters is how many chunks
// of each identifier there are and what each one holds.
func (cfg compareConfig) appendChunkDifferences(diffs []Difference, a, b []Chunk) []Difference {
	for _, id := range chunkIDs(a, b) {
		if cfg.ignoredChunks[id] {
			continue
		}
		x, y := payloads(a, id), payloads(b, id)
		if len(x) != len(y) {
			diffs = append(diffs, Difference{
				Kind: DiffChunkCount, Frame: noIndex, Offset: noIndex, ID: id,
				A: len(x), B: len(y),
			})
		}
		for i := 0; i < min(len(x), len(y)); i++ {
			if !bytes.Equal(x[i], y[i]) {
				diffs = append(diffs, Difference{
					Kind: DiffChunkData, Frame: noIndex, Offset: i, ID: id,
					A: clone(x[i]), B: clone(y[i]),
				})
			}
		}
	}
	return diffs
}

// chunkIDs returns every identifier either side carries, in order of first
// appearance in a and then in b, so that the differences a comparison reports
// come out in an order that does not depend on map iteration.
func chunkIDs(a, b []Chunk) []ID {
	seen := make(map[ID]bool)
	var ids []ID
	for _, chunks := range [][]Chunk{a, b} {
		for _, c := range chunks {
			if !seen[c.ID] {
				seen[c.ID] = true
				ids = append(ids, c.ID)
			}
		}
	}
	return ids
}

// payloads returns the payload of every chunk with the given identifier, in
// order.
func payloads(chunks []Chunk, id ID) [][]byte {
	var found [][]byte
	for _, c := range chunks {
		if c.ID == id {
			found = append(found, c.Data)
		}
	}
	return found
}

// difference builds a difference that belongs to no frame and to no position
// within a larger value, which is most of them.
func difference(kind DifferenceKind, a, b any) Difference {
	return Difference{Kind: kind, Frame: noIndex, Offset: noIndex, A: a, B: b}
}

// clone copies a payload so that nothing a Difference holds aliases the value it
// was taken from. An empty payload clones to nil, which is the same statement
// about the file and formats the same way.
func clone(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return append([]byte(nil), b...)
}

// byteCount renders a payload as its length, for differences whose contents are
// too long to be worth printing whole.
//
// The fallback covers a Difference a caller built by hand: A and B are declared
// as any, so nothing but this package's own construction keeps the type in step
// with the kind, and printing an unexpected value is better than panicking on it.
func byteCount(v any) string {
	b, ok := v.([]byte)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	if len(b) == 0 {
		return "none"
	}
	return fmt.Sprintf("%d byte(s)", len(b))
}

// hexRun renders a run of differing bytes, abbreviating one too long to read.
// Its fallback is byteCount's, and for the same reason.
func hexRun(v any) string {
	b, ok := v.([]byte)
	if !ok {
		return fmt.Sprintf("%v", v)
	}

	// Eight bytes is enough to recognize a value and short enough that a
	// screenful of differences stays a screenful.
	const shown = 8
	if len(b) <= shown {
		return fmt.Sprintf("% x", b)
	}
	return fmt.Sprintf("% x … (%d byte(s))", b[:shown], len(b))
}
