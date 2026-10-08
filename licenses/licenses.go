// Package licenses provides the license texts of the third-party software
// compiled into goged. Regenerate third_party.txt with
// scripts/gen-licenses.sh after changing dependencies.
package licenses

import _ "embed"

// ThirdParty holds the license texts of the Go standard library and of
// every module goged is built from.
//
//go:embed third_party.txt
var ThirdParty string
