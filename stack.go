// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"encoding/binary"
	"fmt"
)

const (
	// frameHeaderSize is the fixed part of a stack frame: the return PC,
	// the flags byte, the result variable, the arguments-supplied mask, and
	// the evaluation-stack word count.
	frameHeaderSize = 8

	// MaxLocals is the number of local variables a Z-machine routine may
	// have, and therefore the largest count a frame's flags byte can hold.
	MaxLocals = 15

	// MaxEvaluationWords is the largest evaluation stack a single frame can
	// record, since the count is stored in one word.
	MaxEvaluationWords = 0xffff

	// flagLocalCount masks the local-variable count out of the flags byte.
	flagLocalCount = 0x0f

	// flagDiscardResult is the p bit of the flags byte: the call discards
	// its result.
	flagDiscardResult = 0x10

	// flagReserved covers the bits that the flags byte, 000pvvvv, leaves
	// undefined.
	flagReserved = 0xe0

	// argumentsMask covers the seven argument-supplied bits of the
	// arguments byte, 0gfedcba.
	argumentsMask = 0x7f
)

// Frame is one Z-machine call frame as Quetzal records it.
type Frame struct {
	// ReturnPC is the byte address in the story file that the call returns
	// to. Like every Quetzal program counter it is stored in three bytes,
	// so it is never greater than MaxPC.
	ReturnPC uint32

	// DiscardResult reports the p bit: the call was made by one of the
	// CALL_xN instructions and throws its result away. ResultVariable then
	// carries no meaning, and is written as zero.
	DiscardResult bool

	// ResultVariable is the number of the variable the call's result is
	// stored in.
	ResultVariable byte

	// Arguments is the argument-supplied mask, 0gfedcba: bit 0 is set if
	// the first argument was supplied, bit 1 if the second was, and so on
	// through the seven arguments a routine can take. All seven bits are
	// preserved; the eighth is not defined and is never set.
	Arguments uint8

	// Locals holds the routine's local variables in order, so that
	// Locals[0] is local 1. A routine has at most MaxLocals of them.
	Locals []uint16

	// Evaluation is the part of the evaluation stack this call used, in
	// file order: Evaluation[0] is the least recent word and
	// Evaluation[len(Evaluation)-1] is the top of the stack.
	Evaluation []uint16
}

// IsDummy reports whether this is the dummy frame that Quetzal requires as the
// first frame of a save for any Z-machine version other than 6.
//
// Execution in those versions begins at an address rather than at a routine,
// so words can be pushed on the evaluation stack while nothing is on the call
// stack. The dummy frame is where they live: every field is zero except the
// evaluation stack, which may itself be empty.
func (f Frame) IsDummy() bool {
	return f.ReturnPC == 0 && !f.DiscardResult && f.ResultVariable == 0 &&
		f.Arguments == 0 && len(f.Locals) == 0
}

// Validate reports whether the frame can be represented in Quetzal.
func (f Frame) Validate() error {
	if err := f.validate(); err != nil {
		return prefixed(err)
	}
	return nil
}

// validate reports the same problems as Validate, without the package prefix,
// so that a caller can name the frame the problem belongs to.
func (f Frame) validate() error {
	switch {
	case f.ReturnPC > MaxPC:
		return newErr(ErrInvalidFormat,
			"return program counter %#x exceeds the 24-bit maximum %#x", f.ReturnPC, MaxPC)
	case len(f.Locals) > MaxLocals:
		return newErr(ErrInvalidFormat,
			"%d local variables, but the count is held in four bits and cannot exceed %d",
			len(f.Locals), MaxLocals)
	case len(f.Evaluation) > MaxEvaluationWords:
		return newErr(ErrInvalidFormat,
			"%d words of evaluation stack, but the count is held in one word and cannot exceed %d",
			len(f.Evaluation), MaxEvaluationWords)
	case f.Arguments&^argumentsMask != 0:
		return newErr(ErrInvalidFormat,
			"arguments mask %#02x sets a bit outside the seven the format defines", f.Arguments)
	}
	return nil
}

// FrameError identifies which frame of a Stks chunk an error belongs to.
// Index counts from zero at the oldest frame, the order frames are stored in.
type FrameError struct {
	Index int
	Err   error
}

// Error implements the error interface.
func (e *FrameError) Error() string {
	return fmt.Sprintf("quetzal: Stks frame %d: %s", e.Index, e.Err)
}

// Unwrap returns the underlying error so that errors.Is and errors.As reach
// the sentinel it wraps.
func (e *FrameError) Unwrap() error { return e.Err }

// Frames decodes the save's call stack from its Stks chunk, oldest frame
// first. The limits the file was decoded under bound what it may allocate.
func (f *File) Frames() ([]Frame, error) {
	c, ok := f.First(IDStks)
	if !ok {
		return nil, prefixed(newErr(ErrInvalidFormat, "missing %s chunk", IDStks))
	}
	return DecodeStks(c.Data, f.limits)
}

// DecodeStks decodes the payload of a Stks chunk into frames, oldest first.
//
// The dummy frame that versions other than 6 require is returned like any
// other frame rather than being recognized or removed; see Frame.IsDummy and
// ValidateFrames.
//
// Zero-valued fields of limits take their defaults. The payload is neither
// retained nor modified.
func DecodeStks(payload []byte, limits Limits) ([]Frame, error) {
	limits = limits.resolve()

	var frames []Frame
	words := 0
	for off := 0; off < len(payload); {
		index := len(frames)
		if index == limits.MaxFrames {
			return nil, prefixed(newErr(ErrLimitExceeded,
				"Stks: more than %d frames", limits.MaxFrames))
		}

		rest := len(payload) - off
		if rest < frameHeaderSize {
			return nil, &FrameError{Index: index, Err: newErr(ErrTruncated,
				"%d byte(s) left in the chunk, too few for a %d-byte frame header",
				rest, frameHeaderSize)}
		}

		header := payload[off : off+frameHeaderSize]
		flags := header[3]
		if flags&flagReserved != 0 {
			return nil, &FrameError{Index: index, Err: newErr(ErrInvalidFormat,
				"flags byte %#02x sets a bit the format leaves undefined", flags)}
		}
		arguments := header[5]
		if arguments&^argumentsMask != 0 {
			return nil, &FrameError{Index: index, Err: newErr(ErrInvalidFormat,
				"arguments mask %#02x sets a bit outside the seven the format defines", arguments)}
		}

		// A local count is four bits and an evaluation count one word, so
		// the largest frame is well under any integer limit.
		locals := int(flags & flagLocalCount)
		evaluation := int(binary.BigEndian.Uint16(header[6:8]))

		body := 2 * (locals + evaluation)
		if rest-frameHeaderSize < body {
			return nil, &FrameError{Index: index, Err: newErr(ErrTruncated,
				"%d local(s) and %d evaluation word(s) need %d byte(s), but %d remain in the chunk",
				locals, evaluation, body, rest-frameHeaderSize)}
		}

		words += locals + evaluation
		if words > limits.MaxStackWords {
			return nil, prefixed(newErr(ErrLimitExceeded,
				"Stks: more than %d words of local variables and evaluation stack",
				limits.MaxStackWords))
		}

		// Every length is now known to lie within the payload, so these
		// allocations are bounded by input that is already in memory.
		frame := Frame{
			ReturnPC:       be24(header[0:3]),
			DiscardResult:  flags&flagDiscardResult != 0,
			ResultVariable: header[4],
			Arguments:      arguments,
		}
		off += frameHeaderSize
		if locals > 0 {
			frame.Locals = readWords(payload[off:], locals)
			off += 2 * locals
		}
		if evaluation > 0 {
			frame.Evaluation = readWords(payload[off:], evaluation)
			off += 2 * evaluation
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// EncodeStks encodes frames into the payload of a Stks chunk, in the order
// given, which must run from the oldest frame to the newest.
//
// A frame that discards its result has its result variable written as zero,
// since the byte carries no meaning in that case. The frames themselves are
// neither retained nor modified.
func EncodeStks(frames []Frame) ([]byte, error) {
	size := 0
	for i, f := range frames {
		if err := f.validate(); err != nil {
			return nil, &FrameError{Index: i, Err: err}
		}
		size += frameHeaderSize + 2*(len(f.Locals)+len(f.Evaluation))
	}

	payload := make([]byte, 0, size)
	for _, f := range frames {
		flags := byte(len(f.Locals))
		result := f.ResultVariable
		if f.DiscardResult {
			flags |= flagDiscardResult
			result = 0
		}

		var header [frameHeaderSize]byte
		putBE24(header[0:3], f.ReturnPC)
		header[3] = flags
		header[4] = result
		header[5] = f.Arguments
		binary.BigEndian.PutUint16(header[6:8], uint16(len(f.Evaluation)))

		payload = append(payload, header[:]...)
		payload = appendWords(payload, f.Locals)
		payload = appendWords(payload, f.Evaluation)
	}
	return payload, nil
}

// ValidateFrames checks a call stack against the story it belongs to.
//
// Every frame must be representable, and a save for any Z-machine version
// other than 6 must begin with the dummy frame that holds top-level
// evaluation-stack state.
func ValidateFrames(frames []Frame, story Story) error {
	// Version 6 starts at a routine, so its first frame is a real call and
	// no dummy frame is written.
	if story.Version != 6 {
		if len(frames) == 0 {
			return prefixed(newErr(ErrInvalidFormat,
				"Stks: no frames, but a version %d save must begin with the dummy frame",
				story.Version))
		}
		if !frames[0].IsDummy() {
			return &FrameError{Index: 0, Err: newErr(ErrInvalidFormat,
				"a version %d save must begin with the dummy frame", story.Version)}
		}
	}
	for i, f := range frames {
		if err := f.validate(); err != nil {
			return &FrameError{Index: i, Err: err}
		}
	}
	return nil
}

// readWords decodes n big-endian words from the front of b, which the caller
// must have established is long enough.
func readWords(b []byte, n int) []uint16 {
	words := make([]uint16, n)
	for i := range words {
		words[i] = binary.BigEndian.Uint16(b[2*i : 2*i+2])
	}
	return words
}

// appendWords appends words to b in big-endian order.
func appendWords(b []byte, words []uint16) []byte {
	for _, w := range words {
		b = binary.BigEndian.AppendUint16(b, w)
	}
	return b
}
