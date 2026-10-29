package utilitytrace

import "github.com/butler/butler/internal/memory/catalog"

// CurrentDocument is an external read receipt, not a memory-store insertion.
type CurrentDocument struct {
	Ref  catalog.SourceRef `json:"source"`
	Body string            `json:"body"`
}
