package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/butler/butler/apps/tool-broker/internal/app"
)

func main() {
	endpoint := flag.String("control-url", "", "fixed authenticated Core communication endpoint")
	flag.Parse()
	a, err := app.NewForCommunication(context.Background(), *endpoint)
	if err == nil {
		err = a.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolated communication Broker unavailable; inspect private service diagnostics")
		os.Exit(1)
	}
}
