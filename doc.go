// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package quetzal reads, validates, manipulates, and writes Quetzal
// saved-game files for the Z-machine.
//
// Quetzal is a saved-state file format, not a Z-machine. This package does
// not execute Z-machine instructions, implement the object, dictionary, or
// text systems, or emulate a terminal. It models Quetzal as a file format so
// that interpreters, servers, and inspection tools can share one
// implementation.
//
// The implementation follows The Quetzal Z-Machine Saved Game Standard,
// version 1.4, by Martin Frost. A Quetzal file is an IFF FORM whose form type
// is IFZS and whose required contents are an IFhd chunk, one of CMem or UMem,
// and a Stks chunk. All multi-byte integers are big-endian.
//
// # Layers
//
// The package separates the IFF container from the Quetzal payloads:
//
//	Decode  parses the container into raw chunks and needs no story file.
//	Read    additionally reconstructs saved state and requires the story.
//
// The separation matters because inspecting a save's structure, header, or
// annotations does not inherently require the original story image, while
// reconstructing compressed dynamic memory does.
//
// Writing runs the same way in reverse. Save.Encode turns saved state into a
// container, File.WriteTo writes a container out, and Write does both:
//
//	quetzal.Read(r, story)  ==  quetzal.Decode(r) then File.Save(story)
//	quetzal.Write(w, story, save)  ==  Save.Encode(story) then File.WriteTo(w)
//
// Reading and writing round trip semantically rather than byte for byte: a
// save that is read and written again holds the same story identity, dynamic
// memory, program counter, and call stack, but need not be the same sequence
// of bytes, since the format leaves the choice of encoding open.
//
// # Naming
//
// Four prefixes recur, and each means something:
//
//	Parse    turns a payload into one value of fixed layout: ParseHeader,
//	         ParseStory, ParseInterpreterData.
//	Decode   turns a payload into however many values it describes:
//	         Decode, DecodeCMem, DecodeStks.
//	Validate reports whether a value could be written, without writing it.
//	         Header, Frame, Memory, and Save each have one; ValidateFrames
//	         checks a whole call stack against its story.
//	Compare  reports how two values of one kind differ, judging neither:
//	         Compare over saves, CompareFiles over containers.
//
// Limits bounds the calls where, and only where, the input decides how much
// there is to allocate: Decode, which reads a chunk count out of the FORM, and
// DecodeStks, which reads frame and word counts out of its payload. DecodeCMem
// needs none, because its result is exactly as long as the original memory the
// caller supplied, however long the difference stream turns out to be. Decode
// takes its limits through WithLimits, since it accepts other options too;
// DecodeStks takes a Limits directly, since none of the other options mean
// anything to a bare payload.
//
// Encode is the inverse of the layer it is called on rather than of any one
// of these: Header.Encode returns a payload, Memory.Encode returns a Chunk,
// and Save.Encode returns a whole File.
//
// # Options
//
// ReadOption, WriteOption, and CompareOption configure a call. All three are
// functions over an unexported type, so this package defines every option there
// is and a caller cannot write its own.
//
// For ReadOption and WriteOption that closure is the point: an option there
// names one rule being relaxed or one choice being made, and the set of rules is
// the format's rather than open-ended. IgnoreChunkOrder exists because Quetzal
// states an ordering rule that not every interpreter enforces. There is no
// option to accept a save for the wrong story, because no such leniency is
// defensible.
//
// CompareOption is closed for a weaker reason, and the difference is worth
// stating. What a caller is willing to disregard when it compares two saves is
// its own testing policy, not a rule of the format, so the reasoning above does
// not reach it: IgnoreMemoryRange takes arbitrary bounds precisely because
// nothing in Quetzal says which bytes of dynamic memory a caller ought to care
// about. These options are a closed set only because a caller that needs
// another one is better served by asking for it than by writing it — an option
// that ships here is documented, tested, and named for what it disregards, and
// the next caller finds it. See Compare.
//
// # Comparison
//
// Compare reports how two saves differ, and CompareFiles does the same for two
// containers. Neither is part of the Quetzal format: they exist to make a
// difference between this package and another interpreter, or between two runs
// of one interpreter, something a test can print rather than something a person
// has to find in a byte dump.
//
// They are therefore outside the scope of specification.md, which describes the
// format and this package's reading and writing of it. That exclusion is stated
// in its §5.7, along with the requirements a facility of this kind must still
// meet. The design is recorded in
// https://github.com/maloquacious/quetzal/issues/1, and these doc comments are
// authoritative for the behavior.
//
// # Story data
//
// Compressed memory (CMem) is an XOR difference against the story's original
// dynamic memory, so it cannot be reconstructed without that memory. This
// package never searches the filesystem for a matching story file and never
// performs filesystem or network access as a side effect of parsing. Callers
// supply story data explicitly.
//
// # Stored files
//
// A file this package writes stays readable by later releases. The format on
// disk is Quetzal 1.4's rather than this package's, and nothing the writer
// emits records a package version, so no change to this API can reach bytes
// already saved. That holds before v1.0 as well as after: the caveat that a
// package below v1.0 may still change is a caveat about this API, and no stored
// file depends on it. Version reports the package version and the Quetzal
// version separately because the two do not move together.
//
// A caller persisting saves should not assume two things. A save is not
// self-contained: it names its story by release number, serial number, and
// checksum, and compressed memory is a difference against that story's dynamic
// memory, so reading one back requires the same story. And rewriting a save
// does not reproduce its bytes, because a round trip preserves the state rather
// than the encoding, so a hash of the file does not identify the position it
// holds.
//
// The policy, including what would have to justify a later release rejecting a
// file an earlier one accepted, is
// https://github.com/maloquacious/quetzal/blob/main/specification.md#261-files-already-written.
//
// # Untrusted input
//
// Saved games are binary input from untrusted sources. Every length field in
// a Quetzal file is attacker-controlled, so this package bounds-checks reads,
// uses overflow-safe arithmetic, validates lengths before allocating, and
// enforces configurable resource limits. Malformed input returns an error
// rather than panicking.
package quetzal
