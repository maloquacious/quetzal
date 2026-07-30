// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"testing"

	"github.com/maloquacious/quetzal"
)

// TestVersion pins the reported versions. Bumping either constant should be a
// deliberate change, so this test is expected to be updated along with it.
func TestVersion(t *testing.T) {
	const (
		wantPkg  = "0.2.3"
		wantSpec = "1.4"
	)

	pkg, spec := quetzal.Version()
	if pkg != wantPkg {
		t.Errorf("package version: got %q, want %q", pkg, wantPkg)
	}
	if spec != wantSpec {
		t.Errorf("spec version: got %q, want %q", spec, wantSpec)
	}
}
