package actionauth

import "time"

// AuthorityDeadline is a transient release bound. It never replaces the
// mandate's immutable budget period or replenishes a persisted scope.
func AuthorityDeadline(m Mandate, g GoalPolicy, at time.Time) (time.Time, error) {
	deadline := m.ExpiresAt
	for _, candidate := range []time.Time{m.BudgetDeadline, g.ExpiresAt} {
		if candidate.IsZero() {
			return time.Time{}, ErrOwnerAuthority
		}
		if candidate.Before(deadline) {
			deadline = candidate
		}
	}
	for _, policy := range g.Policies {
		if policy.ExpiresAt.IsZero() {
			return time.Time{}, ErrOwnerAuthority
		}
		if policy.ExpiresAt.Before(deadline) {
			deadline = policy.ExpiresAt
		}
	}
	if m.Hours != nil {
		loc, err := time.LoadLocation(m.Hours.Timezone)
		if err != nil || !withinHours(m.Hours, loc, at) {
			return time.Time{}, ErrOwnerAuthority
		}
		// Walk actual minute boundaries, rather than assuming every local day has
		// 24 hours. This preserves DST folds/gaps and the selected-calendar-day rule.
		boundary := at.Truncate(time.Minute).Add(time.Minute)
		found := false
		for i := 0; i < 48*60; i++ {
			if !withinHours(m.Hours, loc, boundary) {
				if boundary.Before(deadline) {
					deadline = boundary
				}
				found = true
				break
			}
			boundary = boundary.Add(time.Minute)
		}
		if !found {
			return time.Time{}, ErrOwnerAuthority
		}
	}
	if deadline.IsZero() || !deadline.After(at) {
		return time.Time{}, ErrOwnerAuthority
	}
	return deadline, nil
}
