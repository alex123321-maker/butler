//go:build !linux

package acp

import "context"

func (probe *Probe) RunProcess(parent context.Context, spec ProcessSpec, request Request) (Result, error) {
	return Result{}, ErrProcessUnsupported
}
