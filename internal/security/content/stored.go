package content

import "encoding/json"

// StoredDataOrigin preserves source references, never a stored claim of
// instruction authority or public sensitivity. Legacy material is private.
func StoredDataOrigin(kind Kind, ref, provenance string) Origin {
	o := Origin{Kind: kind, SourceRefs: []string{ref}, Sensitivity: Private}
	var parent Origin
	if json.Unmarshal([]byte(provenance), &parent) == nil {
		o.ParentRefs = append(parent.SourceRefs, parent.ParentRefs...)
		if parent.Sensitivity == Secret {
			o.Sensitivity = Secret
		}
	}
	if o.IsInstruction() || kind == OwnerInput {
		o.Kind = MemoryDerived
	}
	return o.normalized()
}
