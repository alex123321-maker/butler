package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/butler/butler/internal/memory/memorytest"
)

func TestTemporalLifecycleAttackDataCannotAssertValidityOrFreshnessIntegration(t *testing.T) {
	s, owner := fixture(t)
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			src := put(t, s, owner, "temporal-attack-"+attack.ID, attack.Body)
			c := propose(t, s, owner, src, `"high-confidence DATA"`)
			if c.Status != "candidate" {
				t.Fatal("attack confidence became acceptance")
			}
			if _, err := s.SetTemporal(context.Background(), TemporalUpdate{ClaimID: c.ID, Validity: ValidityInterval, Class: FreshnessStablePreference, OwnerVerifiedNow: true}); !errors.Is(err, ErrDenied) {
				t.Fatal("stored instructions asserted owner validity", err)
			}
			if _, err := s.SetTemporalFromBrokerRead(context.Background(), BrokerTemporalUpdate{ClaimID: c.ID, ConfirmedSameFact: true}); !errors.Is(err, ErrDenied) {
				t.Fatal("quoted evidence minted owner review", err)
			}
			page, err := s.ReadAcceptedFacts(owner, []string{c.ID}, TemporalQuery{})
			if err != nil || len(page.Items) != 0 {
				t.Fatal("candidate became accepted temporal fact", page, err)
			}
			if _, err = s.Delete(owner, src.ID, "delete", src.Revision); err != nil {
				t.Fatal(err)
			}
		})
	}
}
