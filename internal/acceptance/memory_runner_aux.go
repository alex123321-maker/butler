package acceptance

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Corroborate the actual embedding input, not merely its request hash: query
// is the reviewed question; each document text is the owned catalog fragment.
func memoryAuditAuxAllowed(s MemorySuite, caseID string, raw json.RawMessage, ledger map[string]json.RawMessage) error {
	var request struct {
		ID      string `json:"request_id"`
		Profile string `json:"profile_id"`
		Caller  string `json:"caller_id"`
		Input   struct {
			Embed struct {
				Task       int `json:"task"`
				Dimensions int `json:"dimensions"`
				Texts      []struct {
					ID    string `json:"id"`
					Text  string `json:"text"`
					Title string `json:"title"`
				} `json:"texts"`
			}
		}
	}
	if json.Unmarshal(raw, &request) != nil || request.ID == "" || request.Profile != "embeddinggemma2-text-bf16-v1" || request.Input.Embed.Dimensions != 768 || len(request.Input.Embed.Texts) < 1 || len(request.Input.Embed.Texts) > 4 {
		return errors.New("actual auxiliary profile/input differs")
	}
	var c MemoryCase
	for _, candidate := range s.Cases {
		if MemoryOpaqueCaseID(candidate) == caseID {
			c = candidate
		}
	}
	if c.ID == "" || c.Blocked() {
		return errors.New("foreign/blocked auxiliary input")
	}
	if request.Caller == "memory-query" && request.Input.Embed.Task == 1 {
		if len(request.Input.Embed.Texts) != 1 || request.Input.Embed.Texts[0].Text != c.Question {
			return errors.New("actual query differs")
		}
		return nil
	}
	if request.Caller != "memory-index" || request.Input.Embed.Task != 2 {
		return errors.New("undeclared auxiliary operation")
	}
	var chunks []struct {
		ID    int64  `json:"id"`
		Scope string `json:"scope_id"`
		Title string `json:"title"`
		Body  string `json:"content"`
	}
	var bindings []struct {
		ID     int64  `json:"object_id"`
		Source string `json:"source_id"`
	}
	var sources []struct {
		ID       string `json:"source_id"`
		Owner    string `json:"owner_id"`
		External string `json:"external_key"`
		Status   string `json:"status"`
	}
	if json.Unmarshal(ledger["memory_chunks"], &chunks) != nil || json.Unmarshal(ledger["memory_object_sources"], &bindings) != nil || json.Unmarshal(ledger["memory_sources"], &sources) != nil {
		return errors.New("auxiliary original binding unavailable")
	}
	for _, text := range request.Input.Embed.Texts {
		chunkID, err := strconv.ParseInt(text.ID, 10, 64)
		if err != nil {
			return errors.New("auxiliary chunk ID invalid")
		}
		matched := false
		for _, chunk := range chunks {
			if chunk.ID == chunkID && chunk.Scope == "memory-eval-owner:"+caseID && chunk.Title == text.Title && chunk.Body == text.Text {
				for _, b := range bindings {
					if b.ID == chunkID {
						for _, src := range sources {
							if src.ID == b.Source && src.Owner == chunk.Scope && src.Status == "active" {
								for _, h := range c.History {
									if h.ID == src.External && h.Status != "deleted" && strings.Contains(h.Body, text.Text) {
										matched = true
									}
								}
							}
						}
					}
				}
			}
		}
		if !matched {
			return errors.New("actual document embedding input lacks approved original")
		}
	}
	return nil
}
