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

// Chunk is one IFF chunk: a four-byte identifier and its exact payload.
//
// Data never includes the pad byte that follows an odd-length chunk. That byte
// is structural, belongs to the container rather than the chunk, and is
// regenerated on write.
type Chunk struct {
	ID   ID
	Data []byte
}
