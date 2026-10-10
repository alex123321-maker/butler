package chunks

import (
	"fmt"
	"github.com/butler/butler/internal/security/content"
)

func (e SearchResult) MemoryOrigin() content.Origin {
	return content.StoredDataOrigin(content.Document, fmt.Sprintf("memory:chunk:%d", e.ID), e.ProvenanceJSON)
}
