package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type behavior76LiteralOpportunity struct {
	EpisodeID   string `json:"episode_id"`
	MaskedArmID string `json:"masked_arm_id"`
	StepID      string `json:"step_id"`
}

type behavior76LiteralPlan struct {
	Version              string                         `json:"version"`
	Episodes             int                            `json:"episodes"`
	Arms                 int                            `json:"arms"`
	OwnerOpportunities   int                            `json:"owner_opportunities"`
	Dimensions           int                            `json:"dimensions"`
	PlannedOpportunities []behavior76LiteralOpportunity `json:"planned_opportunities"`
}

func behavior76LiteralDeclaredPlan(values []behavior76LiteralEpisode, arms []string) (*behavior76LiteralPlan, error) {
	if len(arms) != 2 || arms[0] == "" || arms[1] == "" || arms[0] == arms[1] {
		return nil, errors.New("two distinct masked arms required")
	}
	// Revalidate the complete public interface; callers cannot supply a smaller
	// denominator or a partial set after observing a failure.
	if _, err := behavior76ReadLiteralEpisodes(strings.NewReader(behavior76LiteralInterfaceJSON(values))); err != nil {
		return nil, err
	}
	plan := &behavior76LiteralPlan{Version: behavior76LiteralVersion, Episodes: 16, Arms: 32, OwnerOpportunities: 68, Dimensions: 8}
	for n, value := range values {
		for _, arm := range []string{arms[n%2], arms[(n+1)%2]} {
			for _, step := range value.Episode.Steps {
				if step.Evaluate {
					plan.PlannedOpportunities = append(plan.PlannedOpportunities, behavior76LiteralOpportunity{EpisodeID: value.Episode.ID, MaskedArmID: arm, StepID: step.ID})
				}
			}
		}
	}
	if len(plan.PlannedOpportunities) != 68 {
		return nil, errors.New("literal full denominator unavailable")
	}
	return plan, nil
}

func behavior76LiteralBindingsFresh() bool {
	if !behavior76LiteralBindingsValid(behavior76LiteralStimuliSHA, behavior76LiteralExpectationsSHA, true) {
		return false
	}
	closed := []string{behavior76StimuliSHA, behavior76ExpectationsSHA, behavior76FreshStimuliSHA, behavior76FreshExpectationsSHA, behavior76LocalStimuliSHA, behavior76LocalExpectationsSHA, behavior76StreamStimuliSHA, behavior76StreamExpectationsSHA, behavior76DayStimuliSHA, behavior76DayExpectationsSHA,
		"078673c7da9df829f066faf1c3d1c3b21db716b43f74c4da8b80df2a3272b6a4", "ba633aac4cb8bfb89e55bc16b4396296f0481f4c924fe3383612ac58a4ca9642",
		"bb40fdc7ca4c22c15230364445d0c1e74384392cc9d55ab53ea8aa42a32e5686", "fc202c9b27a025344156273c73ba2bb86c62d1ffed593f18c928668c8abd2c15",
		"ae86f66502969e359cebe7c814ed384be82e7685606dfae6044faa83bde18d00", "2364bbe133f431d505fbf6a57de48764ec0cb3d12c6130d5bb389197f4124bf0",
		"a2d7ffa28a497a72246ba608d1a18351f316c6804a581784a1f76306975ba047", "dabe70e665b96396bb1e3e9bfdbf7503ba2d07dead08482f6ce50d1d19708cc7",
		"628eb9293577e9a37148367c8ae29e8f77b2807d9adc5f92128f910fc7cd3ba", "d6146f7c01ed7c120d1a0fdc7b435a9a52d07a8f15cda1823b20833e2014d0e9"}
	for _, sha := range closed {
		if behavior76LiteralStimuliSHA == sha || behavior76LiteralExpectationsSHA == sha {
			return false
		}
	}
	return true
}

func TestBehavior76LiteralFixedPlanAndUnboundAdmission(t *testing.T) {
	values := behavior76PublicLiteralInterface()
	plan, err := behavior76LiteralDeclaredPlan(values, []string{"public-arm-one", "public-arm-two"})
	if err != nil || plan.Episodes != 16 || plan.Arms != 32 || plan.OwnerOpportunities != 68 || len(plan.PlannedOpportunities) != 68 {
		t.Fatal("fixed literal plan unavailable", err)
	}
	seen := map[behavior76LiteralOpportunity]bool{}
	for _, opportunity := range plan.PlannedOpportunities {
		if seen[opportunity] {
			t.Fatal("duplicate planned opportunity")
		}
		seen[opportunity] = true
	}
	if _, err = behavior76LiteralDeclaredPlan(values[:15], []string{"public-arm-one", "public-arm-two"}); err == nil {
		t.Fatal("partial plan accepted")
	}
	if _, err = behavior76LiteralDeclaredPlan(values, []string{"same", "same"}); err == nil {
		t.Fatal("unpaired plan accepted")
	}
	if behavior76LiteralStimuliSHA == "" && behavior76LiteralBindingsFresh() {
		t.Fatal("unbound controls permit new study")
	}
}

func TestBehavior76NativeLiteralSupplement(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "literal-supplement-v11-synthetic-only" {
		t.Skip("separate literal one-use admission required")
	}
	if !behavior76LiteralBindingsFresh() {
		t.Fatal("fresh literal controls not bound; no new native admission")
	}
	if err := behavior76LiteralCertificateStart(time.Now().UTC(), os.Getenv("BUTLER_BEHAVIOR76_LITERAL_NOT_BEFORE_UTC"), os.Getenv("BUTLER_BEHAVIOR76_LITERAL_LATEST_START_UTC")); err != nil {
		t.Fatal("finite literal certificate entry expired or unavailable")
	}
	behavior76CollectStudyPlan(t, true, true, true, true)
}

// A host precheck does not cover WSL startup delay. Actual ELF entry and actual
// original window start both independently deny late/unknown certificate time.
// This never extends the original four-hour context or any owning deadline.
func behavior76LiteralCertificateStart(now time.Time, notBeforeText, latestText string) error {
	before, e1 := time.Parse(time.RFC3339Nano, notBeforeText)
	latest, e2 := time.Parse(time.RFC3339Nano, latestText)
	if e1 != nil || e2 != nil || !strings.HasSuffix(notBeforeText, "Z") || !strings.HasSuffix(latestText, "Z") || now.IsZero() || now.Location() != time.UTC {
		return errors.New("actual UTC certificate time required")
	}
	day := now.Truncate(24 * time.Hour)
	if before.Before(day) || !before.Before(day.Add(24*time.Hour)) || latest.Before(before) || latest.Sub(before) > 30*time.Minute || latest.After(day.Add(20*time.Hour)) || now.Before(before) || now.After(latest) {
		return errors.New("literal admission outside finite same-day start interval")
	}
	return nil
}

func TestBehavior76LiteralCertificateActualStart(t *testing.T) {
	before := "2026-10-27T01:00:00.0000000Z"
	latest := "2026-10-27T01:20:00.0000000Z"
	now := time.Date(2026, 10, 27, 1, 10, 0, 0, time.UTC)
	if err := behavior76LiteralCertificateStart(now, before, latest); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		now            time.Time
		before, latest string
	}{
		{now, before, ""}, {now, "", latest}, {now, before, "2026-10-27T01:20:00+00:00"},
		{now.Add(-11 * time.Minute), before, latest}, {now.Add(11 * time.Minute), before, latest},
		{now, before, "2026-10-27T02:00:00Z"}, {now, before, "2026-10-28T01:20:00Z"},
		{time.Date(2026, 10, 27, 19, 55, 0, 0, time.UTC), "2026-10-27T19:50:00Z", "2026-10-27T20:01:00Z"},
	} {
		if behavior76LiteralCertificateStart(sample.now, sample.before, sample.latest) == nil {
			t.Fatal("unknown/early/expired/extended/day-crossing actual start admitted")
		}
	}
}
