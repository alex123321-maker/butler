// memory-admin performs explicit operator initialization of the deletion anchor.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/butler/butler/internal/memory/catalog"
)

func main() {
	path := flag.String("initialize-manifest", "", "absolute new deletion anchor path; never replace an existing anchor")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: memory-admin -initialize-manifest /persistent-private/deletions.log")
		os.Exit(2)
	}
	if e := catalog.NewFileManifest(*path).Initialize(); e != nil {
		fmt.Fprintln(os.Stderr, "deletion anchor initialization failed; existing or invalid anchors are never replaced")
		os.Exit(1)
	}
	fmt.Println("deletion anchor initialized")
}
