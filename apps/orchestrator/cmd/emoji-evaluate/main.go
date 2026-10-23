package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"os"
)

// Offline single-rater evaluation; no network, database or model required.
func main() {
	d := telegramrich.NewDecorator()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var q struct {
			ID       string `json:"id"`
			Text     string `json:"text"`
			Expected string `json:"expected"`
			Split    string `json:"split"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &q); err != nil {
			panic(err)
		}
		v := d.Select(q.Text)
		raw, _ := json.Marshal(struct {
			ID       string                     `json:"id"`
			Split    string                     `json:"split"`
			Expected string                     `json:"expected"`
			Decision telegramrich.EmojiDecision `json:"decision"`
			Correct  bool                       `json:"correct"`
		}{q.ID, q.Split, q.Expected, v, v.ID == q.Expected})
		fmt.Println(string(raw))
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
