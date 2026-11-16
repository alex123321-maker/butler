//go:build !linux

package acp

func CheckRuntimeNamespace() error { return ErrProcessUnsupported }
func KillRuntimeChildren() error   { return ErrProcessUnsupported }
