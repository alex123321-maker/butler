package episodic

import (
	"fmt"
	"github.com/butler/butler/internal/security/content"
)

func (e SearchResult) MemoryOrigin() content.Origin {
	return content.StoredDataOrigin(content.MemoryDerived, fmt.Sprintf("memory:episodic:%d", e.ID), e.ProvenanceJSON)
}

func (e Episode) MemoryOrigin() content.Origin {
	return content.StoredDataOrigin(content.MemoryDerived, fmt.Sprintf("memory:episodic:%d", e.ID), e.ProvenanceJSON)
}
