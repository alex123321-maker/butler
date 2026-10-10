package browserscripts

import _ "embed"

// Runtime is part of the reviewed binary; external helper updates cannot change
// the executable code between verification and process startup.
//
//go:embed browser_runtime.mjs
var Runtime string
