//go:build !linux

package main

import "log"

func main() { log.Fatal("ACP runtime requires Linux Docker isolation") }
