package actionauth

import (
	"errors"
	"testing"
	"time"
)

func TestAuthorityDeadlineClampsTransientPolicyWithoutChangingBudget(t *testing.T) {
	at := time.Date(2026, 10, 12, 10, 30, 17, 0, time.UTC)
	m := testOwnerMandate(at)
	g := GoalPolicy{ExpiresAt: at.Add(12 * time.Hour), Policies: []GoalPolicy{{ExpiresAt: at.Add(90 * time.Second)}}}
	immutable := m.BudgetDeadline
	d, err := AuthorityDeadline(m, g, at)
	if err != nil || !d.Equal(g.Policies[0].ExpiresAt) || !m.BudgetDeadline.Equal(immutable) {
		t.Fatal("transient policy reset budget period", d, err)
	}
	g.Policies[0].ExpiresAt = at
	if _, err = AuthorityDeadline(m, g, at); !errors.Is(err, ErrOwnerAuthority) {
		t.Fatal("expired policy supplied release deadline", err)
	}
	g.Policies = nil
	m.ExpiresAt = time.Time{}
	if _, err = AuthorityDeadline(m, g, at); !errors.Is(err, ErrOwnerAuthority) {
		t.Fatal("unbounded mandate supplied release deadline", err)
	}
}

func TestAuthorityDeadlineUsesActualDSTAndCalendarBoundaries(t *testing.T) {
	cases := []struct {
		name, at, end string
		hours         ActiveHours
	}{
		{"spring_gap", "2026-03-08T06:45:31Z", "2026-03-08T07:00:00Z", ActiveHours{Timezone: "America/New_York", StartMinute: 60, EndMinute: 180, Weekdays: []int{0}}},
		{"fall_fold", "2026-11-01T05:45:31Z", "2026-11-01T07:00:00Z", ActiveHours{Timezone: "America/New_York", StartMinute: 60, EndMinute: 120, Weekdays: []int{0}}},
		{"calendar_day", "2026-10-12T23:30:31Z", "2026-10-13T00:00:00Z", ActiveHours{Timezone: "UTC", StartMinute: 1380, EndMinute: 60, Weekdays: []int{1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, _ := time.Parse(time.RFC3339, c.at)
			end, _ := time.Parse(time.RFC3339, c.end)
			m := testOwnerMandate(at)
			m.Hours = &c.hours
			d, err := AuthorityDeadline(m, GoalPolicy{ExpiresAt: at.Add(24 * time.Hour)}, at)
			if err != nil || !d.Equal(end) {
				t.Fatal("incorrect calendar release bound", d, err, end)
			}
			if _, err = AuthorityDeadline(m, GoalPolicy{ExpiresAt: at.Add(24 * time.Hour)}, end); !errors.Is(err, ErrOwnerAuthority) {
				t.Fatal("closed hours released action", err)
			}
		})
	}
}
