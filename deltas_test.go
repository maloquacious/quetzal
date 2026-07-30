// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The specification is accepted (specification.md §31), which means the two
// documents no longer say different things on purpose. spec-deltas.md records
// where this package departs from Quetzal 1.4, and specification.md §2.1
// enumerates those departures normatively. Nothing keeps the two in step except
// a person remembering to, so these tests check what can be checked
// mechanically: that every delta says what was done with it, that the sections
// it names exist, that every divergence reaches §2.1, and that an identifier
// cited from Go source still resolves.
//
// What none of this checks is whether a fate is true. A wrong annotation passes
// here exactly as a right one does.

const (
	deltasFile = "spec-deltas.md"
	specFile   = "specification.md"
)

var (
	// "### D26 — Limits.MaxUnknownBytes is declared but never enforced"
	entryRe = regexp.MustCompile(`(?m)^### (D\d+) `)

	// "*Fate:* **absorbed** into §16.1, ..." — the fate word is what matters.
	fateRe = regexp.MustCompile(`^\*Fate:\* \*\*(\w+)\*\*`)

	// "## 4. Data model deviations" and "### 9.2.1 What the data model holds".
	// Top-level headings punctuate the number and subsections do not, so the
	// period is optional — but the space after it is not, or "2.1" would match
	// as "2" with the period consumed.
	specHeadingRe = regexp.MustCompile(`(?m)^#{2,3} (\d+(?:\.\d+)*)\.? `)

	// A section reference in prose: §16.1, §5, §2.1.
	sectionRefRe = regexp.MustCompile(`§(\d+(?:\.\d+)*)`)

	// A delta cited from Go source or from prose. Bounded so that a stray
	// "D3" inside an identifier does not match.
	citationRe = regexp.MustCompile(`\bD(\d{1,2})\b`)
)

// fates are the four things that can have been done with a delta. They come
// from the triage that accepted the specification; spec-deltas.md's preamble
// defines each one.
var fates = map[string]bool{
	"absorbed":   true, // the specification now states this normatively
	"divergence": true, // a departure from Quetzal 1.4, enumerated in §2.1
	"limitation": true, // an accepted gap, recorded in §30
	"resolved":   true, // a gap later work closed
}

type delta struct {
	id   string // "D26"
	fate string // "absorbed"
	body string // everything from the heading to the next one
}

func readDeltas(t *testing.T) []delta {
	t.Helper()

	text := readDoc(t, deltasFile)
	locs := entryRe.FindAllStringSubmatchIndex(text, -1)
	if len(locs) == 0 {
		t.Fatalf("%s: found no delta entries; has the heading format changed?", deltasFile)
	}

	deltas := make([]delta, 0, len(locs))
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		d := delta{
			id:   text[loc[2]:loc[3]],
			body: text[loc[0]:end],
		}
		for _, line := range strings.Split(d.body, "\n") {
			if m := fateRe.FindStringSubmatch(line); m != nil {
				d.fate = m[1]
				break
			}
		}
		deltas = append(deltas, d)
	}
	return deltas
}

func readDoc(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestEveryDeltaHasAFate is the structural check the rest depend on. An entry
// added without triage is the failure it exists to catch: a delta nobody has
// decided about reads, months later, exactly like one that was accepted.
func TestEveryDeltaHasAFate(t *testing.T) {
	for _, d := range readDeltas(t) {
		if d.fate == "" {
			t.Errorf("%s carries no *Fate:* line; see the preamble of %s for the four it may have",
				d.id, deltasFile)
			continue
		}
		if !fates[d.fate] {
			t.Errorf("%s has fate %q, which is not one of the four: %s",
				d.id, d.fate, strings.Join(sortedKeys(fates), ", "))
		}
	}
}

// TestDeltaIdentifiersAreUnique guards the promise that an identifier is never
// reused. A reference from a commit message or a test name is only worth
// anything if it still lands on the entry it was written against.
func TestDeltaIdentifiersAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, d := range readDeltas(t) {
		if seen[d.id] {
			t.Errorf("%s appears more than once; identifiers are never reused", d.id)
		}
		seen[d.id] = true
	}
}

// TestEveryDivergenceIsInTheSpec is the load-bearing one. §2.1 says the set of
// departures from Quetzal 1.4 is closed and enumerated there, so a divergence
// recorded only in spec-deltas.md would make the specification's own conformance
// claim false — which is the exact failure accepting the specification was meant
// to rule out.
func TestEveryDivergenceIsInTheSpec(t *testing.T) {
	cited := citedIn(sectionOf(t, specFile, "2.1"))

	for _, d := range readDeltas(t) {
		if d.fate != "divergence" {
			continue
		}
		if !cited[d.id] {
			t.Errorf("%s is a divergence from Quetzal 1.4 but does not appear in %s §2.1, "+
				"whose enumeration the specification calls closed", d.id, specFile)
		}
	}
}

// TestDeltaFatesNameRealSections catches a fate pointing at a section that was
// renumbered or never written. A fate is a promise that the rule now lives
// somewhere; a dangling reference is a promise that cannot be checked by the
// person who most needs to.
func TestDeltaFatesNameRealSections(t *testing.T) {
	spec := readDoc(t, specFile)

	headings := make(map[string]bool)
	for _, m := range specHeadingRe.FindAllStringSubmatch(spec, -1) {
		headings[m[1]] = true
	}
	if len(headings) < 20 {
		t.Fatalf("%s: found only %d numbered headings; has the format changed?", specFile, len(headings))
	}

	for _, d := range readDeltas(t) {
		for _, line := range strings.Split(d.body, "\n") {
			if !strings.HasPrefix(line, "*Fate:*") {
				continue
			}
			for _, m := range sectionRefRe.FindAllStringSubmatch(line, -1) {
				if !headings[m[1]] {
					t.Errorf("%s's fate names §%s, which %s does not have", d.id, m[1], specFile)
				}
			}
		}
	}
}

// TestCitedDeltasExist walks the other way: a D-number in a doc comment or a
// test name that no longer resolves. Go source is where these citations do the
// most good and where they are least likely to be updated, since nothing about
// editing memory.go suggests checking a markdown file.
func TestCitedDeltasExist(t *testing.T) {
	known := make(map[string]bool)
	for _, d := range readDeltas(t) {
		known[d.id] = true
	}

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("found no Go source files to check")
	}

	for _, name := range sources {
		text := readDoc(t, name)
		for _, m := range citationRe.FindAllStringSubmatch(text, -1) {
			if !known[m[0]] {
				t.Errorf("%s cites %s, which %s does not define", name, m[0], deltasFile)
			}
		}
	}
}

// sectionOf returns the body of a numbered section, from its heading to the
// next heading at the same or a higher level.
func sectionOf(t *testing.T, file, number string) string {
	t.Helper()

	text := readDoc(t, file)
	locs := specHeadingRe.FindAllStringSubmatchIndex(text, -1)
	for i, loc := range locs {
		if text[loc[2]:loc[3]] != number {
			continue
		}
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return text[loc[0]:end]
	}
	t.Fatalf("%s has no §%s", file, number)
	return ""
}

func citedIn(text string) map[string]bool {
	cited := make(map[string]bool)
	for _, m := range citationRe.FindAllStringSubmatch(text, -1) {
		cited[m[0]] = true
	}
	return cited
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
