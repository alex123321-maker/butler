package jobs

import (
	"context"
	"github.com/butler/butler/internal/owneridentity"
	"testing"
)

func TestExactOwnerClaimCannotStealOtherOwnerKindOrPurposeIntegration(t *testing.T) {
	s, _, owner, src := fixture(t)
	r := request(src)
	r.Kind = "ingest"
	r.Purpose = "voice_tts"
	if _, e := s.Enqueue(owner, r); e != nil {
		t.Fatal(e)
	}
	foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "foreign"})
	for _, tc := range []struct {
		ctx           context.Context
		kind, purpose string
	}{{context.Background(), "ingest", "voice_tts"}, {foreign, "ingest", "voice_tts"}, {owner, "index", "voice_tts"}, {owner, "ingest", "memory_ingest"}} {
		if _, ok, _ := s.ClaimOwner(tc.ctx, r.ID, tc.kind, tc.purpose); ok {
			t.Fatal("unrelated job claimed")
		}
	}
	j, ok, e := s.ClaimOwner(owner, r.ID, "ingest", "voice_tts")
	if e != nil || !ok || j.OwnerID != "owner" || j.ID != r.ID {
		t.Fatal(ok, e)
	}
	if _, ok, e := s.ClaimOwner(owner, r.ID, "ingest", "voice_tts"); e != nil || ok {
		t.Fatal("live claim stolen", ok, e)
	}
}
