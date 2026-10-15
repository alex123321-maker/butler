package app

import (
	"encoding/json"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/factcontrol"
	"github.com/butler/butler/internal/memory/profile"
)

// Code-owned release qualification is independent of the operator flag. The
// temporal read/revalidation/restore gate has not yet been measured end to end.
const temporalReleaseQualified = factcontrol.ReleaseQualified

func temporalConfigured(enabled bool) bool { return enabled && temporalReleaseQualified }
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
