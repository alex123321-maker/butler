//go:build !linux

package workspace

import "os"

// The supported runtime baseline is Linux. No in-memory/PID fallback can
// establish ownership across actual processes on unsupported platforms.
func lockDirectory(*os.File) error { return ErrDenied }
