// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"strconv"
)

// ID is an IFF chunk identifier: four ASCII characters in the range 0x20 to
// 0x7E, compared as a simple four-byte equality test and therefore
// case-sensitive.
type ID [4]byte

// Chunk IDs used by Quetzal. The container layer treats every chunk alike;
// these exist so callers and later layers can select chunks without building
// identifiers by hand.
var (
	IDFORM = ID{'F', 'O', 'R', 'M'} // outer IFF chunk
	IDIFZS = ID{'I', 'F', 'Z', 'S'} // Quetzal FORM type

	IDIFhd = ID{'I', 'F', 'h', 'd'} // story identification
	IDCMem = ID{'C', 'M', 'e', 'm'} // compressed dynamic memory
	IDUMem = ID{'U', 'M', 'e', 'm'} // uncompressed dynamic memory
	IDStks = ID{'S', 't', 'k', 's'} // stack frames
	IDIntD = ID{'I', 'n', 't', 'D'} // interpreter-dependent data

	IDANNO = ID{'A', 'N', 'N', 'O'} // annotation text
	IDAUTH = ID{'A', 'U', 'T', 'H'} // author text
	IDCopy = ID{'(', 'c', ')', ' '} // copyright text
)

// String renders the identifier as its four characters. Identifiers holding
// bytes outside the printable ASCII range are quoted instead, so that error
// messages about malformed input stay readable.
func (id ID) String() string {
	for _, c := range id {
		if c < 0x20 || c > 0x7e {
			return strconv.Quote(string(id[:]))
		}
	}
	return string(id[:])
}

// valid reports whether the identifier consists of four printable ASCII
// characters, as required of every IFF chunk ID.
//
// The standard further requires that any spaces be trailing. That rule is not
// enforced: it distinguishes no valid file from any real one, and rejecting an
// otherwise sound save over it would cost interoperability for no gain. Such
// identifiers are preserved exactly as read.
func (id ID) valid() bool {
	for _, c := range id {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// known reports whether this package assigns a meaning to the identifier.
//
// The distinction exists for one reason: an unknown chunk is retained whole
// because §3.4 says to preserve what is not understood, so unknown payloads are
// the part of a file whose size is bounded by nothing but the file itself.
// Limits.MaxUnknownBytes bounds their total, and this is the set it is measured
// against.
func (id ID) known() bool {
	switch id {
	case IDIFhd, IDCMem, IDUMem, IDStks, IDIntD, IDANNO, IDAUTH, IDCopy:
		return true
	}
	return false
}

// Chunk is one IFF chunk: a four-byte identifier and its exact payload.
//
// Data never includes the pad byte that follows an odd-length chunk. That byte
// is structural, belongs to the container rather than the chunk, and is
// regenerated on write.
type Chunk struct {
	ID   ID
	Data []byte
}

// The three chunks Quetzal defines to hold text (7.2) are read with the
// helpers below, on both File and Save.
//
// Two properties are shared by all of them, and are worth stating once.
//
// The text is returned exactly as stored. The format says these chunks hold
// characters in the range 0x20 to 0x7E and nothing else, but that is not
// enforced here: a chunk breaking the rule still carries the text its writer
// meant, and dropping it would discard information for a defect that harms
// nobody at this layer. Callers displaying the result are displaying bytes
// chosen by whoever wrote the file, and should treat them accordingly.
//
// Nothing about restoring a game depends on them. The format is explicit that
// these are user-level information and that an interpreter must not rely on
// their presence or absence (7.6), so a save carrying none of them is in no way
// deficient.

// text returns the payloads of every chunk with the given identifier, in file
// order, as strings.
func text(chunks []Chunk, id ID) []string {
	var found []string
	for _, c := range chunks {
		if c.ID == id {
			found = append(found, string(c.Data))
		}
	}
	return found
}

// firstText returns the payload of the first chunk with the given identifier.
// The format allows only one AUTH and one "(c) " per file, and where it expects
// one, the first is authoritative.
func firstText(chunks []Chunk, id ID) (string, bool) {
	for _, c := range chunks {
		if c.ID == id {
			return string(c.Data), true
		}
	}
	return "", false
}

// Annotations returns the text of every ANNO chunk, in file order.
//
// Multiple annotations are legal and mean exactly that: several separate
// remarks, not one split across chunks. An interpreter might record the score
// and turn count, or its own name and version; the content is entirely up to
// whoever wrote the file.
func (f *File) Annotations() []string { return text(f.Chunks, IDANNO) }

// Author returns the text of the AUTH chunk, which names whoever created the
// file — on a multi-user system, often just a login name.
func (f *File) Author() (string, bool) { return firstText(f.Chunks, IDAUTH) }

// Copyright returns the text of the "(c) " chunk: a copyright date and holder,
// without the symbol itself, which a caller displaying the text supplies. The
// format notes this is unlikely to be useful on a save file.
func (f *File) Copyright() (string, bool) { return firstText(f.Chunks, IDCopy) }

// Annotations returns the text of every ANNO chunk, in file order.
//
// Multiple annotations are legal and mean exactly that: several separate
// remarks, not one split across chunks. An interpreter might record the score
// and turn count, or its own name and version; the content is entirely up to
// whoever wrote the file.
func (s *Save) Annotations() []string { return text(s.Chunks, IDANNO) }

// Author returns the text of the AUTH chunk, which names whoever created the
// file — on a multi-user system, often just a login name.
func (s *Save) Author() (string, bool) { return firstText(s.Chunks, IDAUTH) }

// Copyright returns the text of the "(c) " chunk: a copyright date and holder,
// without the symbol itself, which a caller displaying the text supplies. The
// format notes this is unlikely to be useful on a save file.
func (s *Save) Copyright() (string, bool) { return firstText(s.Chunks, IDCopy) }

// Sizes and flag bits within an IntD payload.
const (
	// intDHeaderSize is the fixed part of an IntD payload: the operating
	// system identifier, the flags byte, the contents identifier, a
	// reserved word, and the interpreter identifier.
	intDHeaderSize = 12

	// intDFlagPosition is the c bit: the contents are meaningful only for
	// the exact saved position stored in the file that carries them.
	intDFlagPosition = 0x01

	// intDFlagMachine is the s bit: the contents are meaningful only on the
	// machine or network the save was made on.
	intDFlagMachine = 0x02
)

// InterpreterData is the fixed header of an IntD chunk, the place Quetzal
// reserves for information one interpreter needs and others need not
// understand.
//
// Data is opaque. This package assigns no meaning to it, because its meaning
// belongs to whoever defined it: the interpreter named by Interpreter, running
// on the system named by OperatingSystem. The reserved word the format places
// before the interpreter identifier is not represented, since it carries no
// information.
type InterpreterData struct {
	// OperatingSystem names the system the data belongs to. Four spaces
	// mean the data is useful to every port of one interpreter.
	OperatingSystem ID

	// Flags is the flags byte, 000000sc. Prefer PositionSpecific,
	// MachineSpecific, and Copyable to testing its bits.
	Flags byte

	// ContentsID says what the data is, within the scope of the operating
	// system and interpreter that defined it.
	ContentsID byte

	// Interpreter names the interpreter the data belongs to. Four spaces
	// mean the data is useful to every interpreter on the named system.
	Interpreter ID

	// Data is the interpreter's own payload, exactly as stored.
	Data []byte
}

// PositionSpecific reports the c flag: the contents describe this saved
// position and no other, so they must not be carried into another save.
func (d InterpreterData) PositionSpecific() bool { return d.Flags&intDFlagPosition != 0 }

// MachineSpecific reports the s flag: the contents, such as a filename or a
// file reference, are meaningful only on the machine or network the save was
// made on.
func (d InterpreterData) MachineSpecific() bool { return d.Flags&intDFlagMachine != 0 }

// Copyable reports whether this chunk may be carried from one save into
// another.
//
// The format forbids copying position-specific contents outright, and forbids
// copying machine-specific contents onto a different system. This package has
// no notion of what system it is running on and so cannot tell a different one
// from the original, which makes the machine-specific case indistinguishable
// from the forbidden one. Copyable therefore answers no to both. A caller that
// does know its own system can recover such a chunk from the decoded File and
// carry it forward deliberately.
func (d InterpreterData) Copyable() bool {
	return d.Flags&(intDFlagPosition|intDFlagMachine) == 0
}

// ParseInterpreterData decodes the fixed header of an IntD payload, leaving
// the remainder opaque.
//
// The returned value owns its data; the payload is neither retained nor
// modified.
func ParseInterpreterData(payload []byte) (InterpreterData, error) {
	if len(payload) < intDHeaderSize {
		return InterpreterData{}, prefixed(newErr(ErrInvalidFormat,
			"IntD: payload is %d byte(s), want at least %d", len(payload), intDHeaderSize))
	}

	d := InterpreterData{
		OperatingSystem: ID(payload[0:4]),
		Flags:           payload[4],
		ContentsID:      payload[5],
		Interpreter:     ID(payload[8:12]),
	}
	if len(payload) > intDHeaderSize {
		d.Data = append([]byte(nil), payload[intDHeaderSize:]...)
	}
	return d, nil
}
