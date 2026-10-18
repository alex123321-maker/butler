package app

import (
	"encoding/json"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/providerfactory"
)

// Code-owned release qualification is independent of the operator flag. The
// Native V9 qualifies explicit dated reads on the measured High profile; this
// does not activate a process flag or qualify natural-language date parsing.
const temporalReleaseQualified = factcontrol.ReleaseQualified

func temporalConfigured(enabled bool) bool { return enabled && temporalReleaseQualified }

// Owner metadata operations do not perform inference. The model-facing
// consumer additionally requires the exact measured provider and matching quote.
func temporalReadConfigured(enabled bool, p providerfactory.Result, b *budget.Controller) bool {
	return temporalConfigured(enabled) && summaryProfileReady(p, b)
}
func temporalExcluded(f catalog.AcceptedFact) bool {
	return f.Freshness.Status == "expired" || f.Freshness.Status == "not_yet_valid"
}
func temporalAnnotation(f catalog.AcceptedFact) string {
	b, _ := json.Marshal(struct {
		Temporal  catalog.TemporalMetadata `json:"temporal"`
		Freshness catalog.Freshness        `json:"freshness"`
	}{f.Temporal, f.Freshness})
	return "\n[Stored temporal evidence; unknown or stale verification is not a current fact: " + string(b) + "]"
}

type temporalProfileEntry struct {
	profile.Entry
	fact catalog.AcceptedFact
}

func (e temporalProfileEntry) ProfileSummary() string {
	return e.Entry.ProfileSummary() + temporalAnnotation(e.fact)
}

type temporalEpisodeEntry struct {
	episodic.Episode
	fact     catalog.AcceptedFact
	distance float64
}

func (e temporalEpisodeEntry) EpisodeSummary() string {
	return e.Episode.Summary + temporalAnnotation(e.fact)
}
func (e temporalEpisodeEntry) EpisodeDistance() float64 { return e.distance }
