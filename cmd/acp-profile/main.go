package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/butler/butler/internal/acp"
)

func main() {
	directory := flag.String("directory", "", "absolute path to a new dedicated Antigravity HOME (never your regular home)")
	flag.Parse()
	if flag.NArg() != 0 || acp.PrepareAntigravityProfile(*directory) != nil {
		fmt.Fprintln(os.Stderr, "ACP profile preparation refused; use a new private dedicated directory")
		os.Exit(1)
	}
	fmt.Println("Dedicated ACP profile prepared. No credentials imported, login or inference performed.")
}
