//go:build !unix

package workspace

import "os"

// No supported link-count proof on other platforms. Production is Linux;
// fail closed rather than silently admitting potentially shared files.
func singleLink(os.FileInfo) bool { return false }
