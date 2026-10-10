package service

import "github.com/butler/butler/internal/security/content"

func memoryOrigin(value any) content.Origin {
	if source, ok := value.(interface{ MemoryOrigin() content.Origin }); ok {
		return content.Derive(content.MemoryDerived, source.MemoryOrigin())
	}
	return content.Origin{Kind: content.LegacyUnknown, Sensitivity: content.Private, SourceRefs: []string{"memory:legacy_unknown"}}
}
