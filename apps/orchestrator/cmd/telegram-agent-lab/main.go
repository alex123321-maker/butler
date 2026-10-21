// telegram-agent-lab starts the real application with an explicit isolated bot
// composition. Provider/service authentication remains in the ordinary system
// configuration. The profile contains only operator metadata and a token ref.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/butler/butler/apps/orchestrator/internal/app"
	"github.com/butler/butler/internal/communicationcontrol"
)

func main() {
	profile := flag.String("profile", "", "private operator profile file")
	flag.Parse()
	if run(*profile) != nil {
		fmt.Fprintln(os.Stderr, "isolated Telegram agent unavailable; inspect private service diagnostics")
		os.Exit(1)
	}
}

func run(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	var options app.TelegramCommunicationOptions
	if err != nil || len(raw) > 8192 || communicationcontrol.StrictJSON(raw, &options) != nil {
		return communicationcontrol.ErrDenied
	}
	a, err := app.NewForTelegramCommunication(context.Background(), options)
	if err != nil {
		return err
	}
	return a.Run(context.Background())
}
