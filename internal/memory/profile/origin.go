package profile

import (
	"fmt"
	"github.com/butler/butler/internal/security/content"
)

func (e Entry) MemoryOrigin() content.Origin {
	return content.StoredDataOrigin(content.MemoryDerived, fmt.Sprintf("memory:profile:%d", e.ID), e.ProvenanceJSON)
}
