// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

// Default resource limits. They are generous relative to real saved games,
// whose dynamic memory cannot exceed 64 KB, while still bounding what a
// hostile length field can cause this package to allocate.
const (
	defaultMaxFormBytes    = 16 << 20 // 16 MiB
	defaultMaxChunkBytes   = 8 << 20  // 8 MiB
	defaultMaxUnknownBytes = 4 << 20  // 4 MiB
	defaultMaxFrames       = 65536
	defaultMaxStackWords   = 1 << 20
)

// Limits bounds the resources a decode may consume. Because every length in a
// Quetzal file is supplied by the file itself, these limits are what stop a
// malformed or hostile input from forcing an unreasonable allocation.
//
// A zero field means "use the default for that field", so a caller may set
// only the limits it cares about. Callers processing trusted historical saves
// normally need no configuration at all.
type Limits struct {
	// MaxFormBytes bounds the declared length of the outer FORM chunk, and
	// with it the total size of the decoded file.
	MaxFormBytes uint64

	// MaxChunkBytes bounds the declared length of any single chunk.
	MaxChunkBytes uint64

	// MaxUnknownBytes bounds the combined payload of the chunks this package
	// assigns no meaning to, which it retains whole rather than discarding.
	// Those are the only chunks whose size nothing but the file itself
	// constrains: every chunk this package understands is bounded by what it
	// can validly contain. Reaching this limit is reported against the chunk
	// that crossed it, before its payload is allocated.
	//
	// Real saves come nowhere near the default. Bocfel's scrollback chunk,
	// the largest unknown chunk seen in practice, is under 3 KB.
	MaxUnknownBytes uint64

	// MaxFrames bounds the number of stack frames read from Stks.
	MaxFrames int

	// MaxStackWords bounds the total number of evaluation-stack and local
	// variable words read from Stks.
	MaxStackWords int
}

// DefaultLimits returns the limits used when a caller supplies none.
func DefaultLimits() Limits {
	return Limits{
		MaxFormBytes:    defaultMaxFormBytes,
		MaxChunkBytes:   defaultMaxChunkBytes,
		MaxUnknownBytes: defaultMaxUnknownBytes,
		MaxFrames:       defaultMaxFrames,
		MaxStackWords:   defaultMaxStackWords,
	}
}

// resolve returns a copy of l with every zero-valued field replaced by its
// default, so that a partially populated Limits behaves predictably.
func (l Limits) resolve() Limits {
	d := DefaultLimits()
	if l.MaxFormBytes == 0 {
		l.MaxFormBytes = d.MaxFormBytes
	}
	if l.MaxChunkBytes == 0 {
		l.MaxChunkBytes = d.MaxChunkBytes
	}
	if l.MaxUnknownBytes == 0 {
		l.MaxUnknownBytes = d.MaxUnknownBytes
	}
	if l.MaxFrames == 0 {
		l.MaxFrames = d.MaxFrames
	}
	if l.MaxStackWords == 0 {
		l.MaxStackWords = d.MaxStackWords
	}
	return l
}
