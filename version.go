// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

// version identifies this package's release, following semantic versioning.
const version = "0.2.3"

// specVersion identifies the version of the Quetzal Z-Machine Saved Game
// Standard that this package implements.
const specVersion = "1.4"

// Version returns the semantic version of this package and the version of the
// Quetzal specification it implements.
func Version() (pkg, spec string) {
	return version, specVersion
}
