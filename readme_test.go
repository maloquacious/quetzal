// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"strings"
	"testing"
)

// TestREADMESnippetsAreCompiled closes the gap specification.md §22 names: the
// README's Go snippets are the first thing a reader tries, and nothing about
// editing this package suggests checking a markdown file. example_test.go holds
// every snippet as an example function, so the compiler checks them; this test
// checks that the README and that file still say the same thing.
//
// Containment runs one way. An example may carry lines the README does not,
// since a snippet in prose can borrow a variable from the section above it while
// a function has to declare one. What is not allowed is a README line that
// nothing compiles.
func TestREADMESnippetsAreCompiled(t *testing.T) {
	readme := readDoc(t, "README.md")
	examples := readDoc(t, "example_test.go")

	compiled := make(map[string]bool)
	for _, line := range strings.Split(examples, "\n") {
		compiled[strings.TrimSpace(line)] = true
	}

	blocks := goBlocks(t, readme)
	if len(blocks) == 0 {
		t.Fatal("README.md: found no Go code blocks; has the fence format changed?")
	}

	for i, block := range blocks {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !compiled[line] {
				t.Errorf("README.md Go block %d has a line that example_test.go does not compile:\n\t%s",
					i+1, line)
			}
		}
	}
}

// goBlocks returns the contents of every ```go fenced block, in order.
func goBlocks(t *testing.T, text string) []string {
	t.Helper()

	var blocks []string
	var current []string
	inside := false

	for _, line := range strings.Split(text, "\n") {
		switch {
		case !inside && strings.TrimSpace(line) == "```go":
			inside = true
			current = nil
		case inside && strings.TrimSpace(line) == "```":
			inside = false
			blocks = append(blocks, strings.Join(current, "\n"))
		case inside:
			current = append(current, line)
		}
	}

	if inside {
		t.Fatal("README.md: a Go code block was never closed")
	}
	return blocks
}
