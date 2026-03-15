package provenance

import "testing"

func TestNormalizeLinkDefaultsMetadata(t *testing.T) {
	t.Parallel()
	link := normalizeLink(Link{SourceMemoryType: " working ", SourceMemoryID: 1, LinkType: "source", TargetType: "run", TargetID: " run-1 "})
	if link.SourceMemoryType != "working" || link.TargetID != "run-1" {
		t.Fatalf("normalizeLink returned %+v", link)
	}
	if link.MetadataJSON != "{}" {
		t.Fatalf("expected default metadata json, got %q", link.MetadataJSON)
	}
}

func TestValidateLinkRequiresFields(t *testing.T) {
	t.Parallel()
	if err := validateLink(Link{}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSaveLinkRequiresStorePool(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).SaveLink(nil, Link{SourceMemoryType: "profile", SourceMemoryID: 1, LinkType: "source", TargetType: "run", TargetID: "run-1"})
	if err != ErrStoreNotConfigured {
		t.Fatalf("expected ErrStoreNotConfigured, got %v", err)
	}
}
