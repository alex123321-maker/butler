// Command extension-manifest generates local reviewed fixture data. It fetches
// no source and requires the operator-measured executor artifact digest.
package main

import (
	"flag"
	"fmt"
	"github.com/butler/butler/internal/extensions/projectfixture"
	"os"
)

func main() {
	digest := flag.String("runtime-sha256", "", "operator-measured executor artifact SHA-256")
	revision := flag.String("source-revision", "", "reviewed source revision")
	flag.Parse()
	p, err := projectfixture.Manifest(*digest, *revision)
	if err != nil {
		fmt.Fprintln(os.Stderr, "valid reviewed revision and runtime digest required")
		os.Exit(1)
	}
	os.Stdout.Write(p.Raw)
	os.Stdout.Write([]byte("\n"))
}
